package engine

import (
	"context"
	"sort"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

// TimelineEntry is a unified chronological item: a commit, event, or link.
type TimelineEntry struct {
	Timestamp   time.Time
	Kind        string // "commit", "event", "link"
	Summary     string
	SHA         string
	EventID     string
	FilePaths   []string
	Identifiers []string
}

// Timeline reconstructs a chronological sequence from commits and events
// related to the given anchor (file path, keyword, or broad query).
func (e *Engine) Timeline(ctx context.Context, anchor string, since time.Time) ([]TimelineEntry, error) {
	var entries []TimelineEntry

	commits, err := e.store.QueryCommits(ctx, kleio.CommitFilter{
		MessageSearch: anchor,
		Since:         formatTime(since),
		Limit:         200,
	})
	if err != nil {
		return nil, err
	}
	for _, c := range commits {
		t, _ := time.Parse(time.RFC3339, c.CommittedAt)
		entry := TimelineEntry{
			Timestamp: t,
			Kind:      "commit",
			Summary:   firstLine(c.Message),
			SHA:       c.SHA,
		}
		entries = append(entries, entry)
	}

	fileCommits, err := e.store.QueryCommits(ctx, kleio.CommitFilter{
		FilePath: anchor,
		Since:    formatTime(since),
		Limit:    200,
	})
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(commits))
	for _, c := range commits {
		seen[c.SHA] = true
	}
	for _, c := range fileCommits {
		if seen[c.SHA] {
			continue
		}
		t, _ := time.Parse(time.RFC3339, c.CommittedAt)
		entries = append(entries, TimelineEntry{
			Timestamp: t,
			Kind:      "commit",
			Summary:   firstLine(c.Message),
			SHA:       c.SHA,
		})
	}

	events, err := e.store.ListEvents(ctx, kleio.EventFilter{Limit: 200})
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		if !containsCI(ev.Content, anchor) && !containsCI(ev.FreeformContext, anchor) {
			continue
		}
		t, _ := time.Parse(time.RFC3339, ev.CreatedAt)
		if !since.IsZero() && t.Before(since) {
			continue
		}
		entries = append(entries, TimelineEntry{
			Timestamp: t,
			Kind:      "event",
			Summary:   firstLine(ev.Content),
			EventID:   ev.ID,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	return entries, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
