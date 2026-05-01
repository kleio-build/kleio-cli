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
//
// When repoName is non-empty, results are restricted to that repository.
// When empty, all repositories are included (the --all-repos behaviour).
func (e *Engine) Timeline(ctx context.Context, anchor string, since time.Time) ([]TimelineEntry, error) {
	return e.TimelineScoped(ctx, anchor, "", since)
}

// TimelineScoped is the repo-aware variant of Timeline. The trace command
// passes the current repo name by default and "" when --all-repos is set.
func (e *Engine) TimelineScoped(ctx context.Context, anchor, repoName string, since time.Time) ([]TimelineEntry, error) {
	expanded := e.expandAnchor(ctx, anchor)
	var entries []TimelineEntry

	commits, err := e.store.QueryCommits(ctx, kleio.CommitFilter{
		MessageSearch: expanded,
		RepoName:      repoName,
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
		RepoName: repoName,
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
		ContentSearch: expanded,
		RepoName:      repoName,
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
	return e.FileTimelineScoped(ctx, path, "", since)
}

// FileTimelineScoped is the repo-aware variant of FileTimeline.
func (e *Engine) FileTimelineScoped(ctx context.Context, path, repoName string, since time.Time) ([]TimelineEntry, error) {
	var entries []TimelineEntry

	commits, err := e.store.QueryCommits(ctx, kleio.CommitFilter{
		FilePath: path,
		RepoName: repoName,
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

	events, err := e.store.ListEvents(ctx, kleio.EventFilter{
		RepoName: repoName,
		Limit:    200,
	})
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

// expandAnchor returns a space-joined query suitable for both
// localdb.QueryCommits (LIKE %word% per token, OR'd) and ListEvents'
// FTS5 path (per-token OR). When no expander is wired we return the
// original anchor unchanged so existing tests/behavior keep working.
func (e *Engine) expandAnchor(ctx context.Context, anchor string) string {
	if e == nil || e.expander == nil || strings.TrimSpace(anchor) == "" {
		return anchor
	}
	terms := e.expander.Expand(ctx, anchor)
	if len(terms) <= 1 {
		return anchor
	}
	return strings.Join(terms, " ")
}
