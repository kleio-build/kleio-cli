package engine

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/entity"
)

var ticketRe = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)

// IsTicketAnchor returns true if the anchor matches a ticket pattern.
func IsTicketAnchor(anchor string) bool {
	return ticketRe.MatchString(strings.TrimSpace(anchor))
}

// EntityQuerier is the subset of localdb.Store needed for entity-aware
// queries. This avoids importing localdb in the engine package.
type EntityQuerier interface {
	FindEntity(ctx context.Context, kind, normalizedLabel string) (*kleio.Entity, error)
	FindEntityByAlias(ctx context.Context, alias string) (*kleio.Entity, error)
	QueryEntityMentions(ctx context.Context, entityID string) ([]kleio.EntityMention, error)
}

// EntityTimelineScoped resolves the anchor to a ticket entity, then
// traverses entity_mentions to reconstruct the full workstream timeline.
// Falls back to keyword search if the entity is not found.
func (e *Engine) EntityTimelineScoped(ctx context.Context, anchor, repoName string, since time.Time) ([]TimelineEntry, error) {
	eq, ok := e.store.(EntityQuerier)
	if !ok {
		return e.TimelineScoped(ctx, anchor, repoName, since)
	}

	normalized := entity.NormalizeLabel(kleio.EntityKindTicket, anchor)
	ent, err := eq.FindEntity(ctx, kleio.EntityKindTicket, normalized)
	if err != nil {
		return e.TimelineScoped(ctx, anchor, repoName, since)
	}
	if ent == nil {
		// Try alias lookup.
		ent, err = eq.FindEntityByAlias(ctx, anchor)
		if err != nil || ent == nil {
			return e.TimelineScoped(ctx, anchor, repoName, since)
		}
	}

	mentions, err := eq.QueryEntityMentions(ctx, ent.ID)
	if err != nil || len(mentions) == 0 {
		return e.TimelineScoped(ctx, anchor, repoName, since)
	}

	var entries []TimelineEntry
	seen := map[string]bool{}

	// Collect evidence IDs and resolve them to timeline entries.
	for _, m := range mentions {
		if seen[m.EvidenceID] {
			continue
		}
		seen[m.EvidenceID] = true

		if m.EvidenceType == kleio.EvidenceTypeCommit {
			commits, err := e.store.QueryCommits(ctx, kleio.CommitFilter{
				MessageSearch: extractSHA(m.EvidenceID),
				RepoName:      repoName,
				Limit:         1,
			})
			if err == nil {
				for _, c := range commits {
					t, _ := time.Parse(time.RFC3339, c.CommittedAt)
					if !since.IsZero() && t.Before(since) {
						continue
					}
					entries = append(entries, TimelineEntry{
						Timestamp: t,
						Kind:      kleio.SignalTypeGitCommit,
						Summary:   firstLine(c.Message),
						SHA:       c.SHA,
					})
				}
			}
		} else {
			ev, err := e.store.GetEvent(ctx, m.EvidenceID)
			if err != nil || ev == nil {
				// EvidenceID may be a source_id, not an event id. Try search.
				events, err := e.store.ListEvents(ctx, kleio.EventFilter{
					ContentSearch: m.Context,
					RepoName:      repoName,
					Limit:         1,
				})
				if err == nil {
					for _, ev := range events {
						t, _ := time.Parse(time.RFC3339, ev.CreatedAt)
						if !since.IsZero() && t.Before(since) {
							continue
						}
						entries = append(entries, TimelineEntry{
							Timestamp: t,
							Kind:      ev.SignalType,
							Summary:   firstLine(ev.Content),
							EventID:   ev.ID,
						})
					}
				}
				continue
			}
			if repoName != "" && ev.RepoName != "" && ev.RepoName != repoName {
				continue
			}
			t, _ := time.Parse(time.RFC3339, ev.CreatedAt)
			if !since.IsZero() && t.Before(since) {
				continue
			}
			entries = append(entries, TimelineEntry{
				Timestamp: t,
				Kind:      ev.SignalType,
				Summary:   firstLine(ev.Content),
				EventID:   ev.ID,
			})
		}
	}

	// Also run the standard keyword search to catch anything not yet
	// entity-linked (backward compatibility).
	kwEntries, err := e.TimelineScoped(ctx, anchor, repoName, since)
	if err == nil {
		for _, entry := range kwEntries {
			key := entry.SHA + entry.EventID
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			entries = append(entries, entry)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	return entries, nil
}

// extractSHA extracts the commit SHA from an evidence ID like "git:repo:sha".
func extractSHA(evidenceID string) string {
	parts := strings.Split(evidenceID, ":")
	if len(parts) >= 3 && parts[0] == "git" {
		return parts[len(parts)-1]
	}
	return evidenceID
}
