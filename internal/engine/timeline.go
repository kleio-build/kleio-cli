package engine

import (
	"context"
	"sort"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

// TimelineEntry is a unified chronological item.
type TimelineEntry struct {
	Timestamp   time.Time
	Kind        string // signal_type: "git_commit", "decision", "work_item", "checkpoint", ...
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
	seen := make(map[string]bool, len(commits)*2)
	for _, c := range commits {
		t, _ := time.Parse(time.RFC3339, c.CommittedAt)
		entries = append(entries, TimelineEntry{
			Timestamp: t,
			Kind:      kleio.SignalTypeGitCommit,
			Summary:   firstLine(c.Message),
			SHA:       c.SHA,
		})
		seen[c.SHA] = true
	}

	fileCommits, err := e.store.QueryCommits(ctx, kleio.CommitFilter{
		FilePath: anchor,
		Since:    formatTime(since),
		Limit:    200,
	})
	if err != nil {
		return nil, err
	}
	for _, c := range fileCommits {
		if seen[c.SHA] {
			continue
		}
		t, _ := time.Parse(time.RFC3339, c.CommittedAt)
		entries = append(entries, TimelineEntry{
			Timestamp: t,
			Kind:      kleio.SignalTypeGitCommit,
			Summary:   firstLine(c.Message),
			SHA:       c.SHA,
		})
		seen[c.SHA] = true
	}

	evFilter := kleio.EventFilter{
		ContentSearch: anchor,
		Limit:         200,
	}
	if !since.IsZero() {
		evFilter.CreatedAfter = formatTime(since)
	}
	events, err := e.store.ListEvents(ctx, evFilter)
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		if ev.SignalType == kleio.SignalTypeGitCommit && strings.HasPrefix(ev.ID, "git:") {
			sha := strings.TrimPrefix(ev.ID, "git:")
			if seen[sha] {
				continue
			}
		}
		t, _ := time.Parse(time.RFC3339, ev.CreatedAt)
		entries = append(entries, TimelineEntry{
			Timestamp: t,
			Kind:      ev.SignalType,
			Summary:   firstLine(ev.Content),
			EventID:   ev.ID,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	return entries, nil
}

// FileTimeline returns the chronological history of a specific file path,
// including all commits that touched it and events referencing it.
func (e *Engine) FileTimeline(ctx context.Context, path string, since time.Time) ([]TimelineEntry, error) {
	var entries []TimelineEntry

	commits, err := e.store.QueryCommits(ctx, kleio.CommitFilter{
		FilePath: path,
		Since:    formatTime(since),
		Limit:    200,
	})
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(commits))
	for _, c := range commits {
		t, _ := time.Parse(time.RFC3339, c.CommittedAt)
		entries = append(entries, TimelineEntry{
			Timestamp: t,
			Kind:      kleio.SignalTypeGitCommit,
			Summary:   firstLine(c.Message),
			SHA:       c.SHA,
			FilePaths: []string{path},
		})
		seen[c.SHA] = true
	}

	events, err := e.store.ListEvents(ctx, kleio.EventFilter{Limit: 200})
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		if ev.SignalType == kleio.SignalTypeGitCommit && strings.HasPrefix(ev.ID, "git:") {
			sha := strings.TrimPrefix(ev.ID, "git:")
			if seen[sha] {
				continue
			}
		}
		if ev.FilePath != path && !containsCI(ev.Content, path) {
			continue
		}
		t, _ := time.Parse(time.RFC3339, ev.CreatedAt)
		entries = append(entries, TimelineEntry{
			Timestamp: t,
			Kind:      ev.SignalType,
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
