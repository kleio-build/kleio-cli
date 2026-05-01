package engine

import (
	"context"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEngine(t *testing.T) (*Engine, *localdb.Store) {
	t.Helper()
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	s := localdb.New(db)
	return New(s, nil), s
}

func TestTimeline_CommitsAndEvents(t *testing.T) {
	eng, store := newTestEngine(t)
	ctx := context.Background()

	now := time.Now().UTC()

	require.NoError(t, store.IndexCommits(ctx, "/repo", []kleio.Commit{
		{
			SHA:         "abc123",
			RepoPath:    "/repo",
			Message:     "feat: add auth module",
			CommittedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
		},
		{
			SHA:         "def456",
			RepoPath:    "/repo",
			Message:     "fix: auth token expiry",
			CommittedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}))

	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:         "evt-1",
		SignalType: kleio.SignalTypeDecision,
		Content:    "decided to use JWT for auth",
		SourceType: kleio.SourceTypeCLI,
		CreatedAt:  now.Add(-90 * time.Minute).Format(time.RFC3339),
	}))

	entries, err := eng.Timeline(ctx, "auth", time.Time{})
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(entries), 3)
	// Should be chronologically ordered
	for i := 1; i < len(entries); i++ {
		assert.False(t, entries[i].Timestamp.Before(entries[i-1].Timestamp),
			"entries should be chronologically ordered")
	}
}

func TestTimeline_FileAnchor(t *testing.T) {
	eng, store := newTestEngine(t)
	ctx := context.Background()

	now := time.Now().UTC()

	require.NoError(t, store.IndexCommits(ctx, "/repo", []kleio.Commit{
		{
			SHA:         "sha1",
			RepoPath:    "/repo",
			Message:     "initial setup",
			CommittedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}))
	require.NoError(t, store.TrackFileChange(ctx, &kleio.FileChange{
		CommitSHA:  "sha1",
		FilePath:   "src/auth.go",
		ChangeType: kleio.ChangeTypeAdded,
	}))

	entries, err := eng.Timeline(ctx, "src/auth.go", time.Time{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 1)
}

func TestTimeline_SinceFilter(t *testing.T) {
	eng, store := newTestEngine(t)
	ctx := context.Background()

	now := time.Now().UTC()

	require.NoError(t, store.IndexCommits(ctx, "/repo", []kleio.Commit{
		{
			SHA:         "old",
			RepoPath:    "/repo",
			Message:     "old auth change",
			CommittedAt: now.Add(-48 * time.Hour).Format(time.RFC3339),
		},
		{
			SHA:         "new",
			RepoPath:    "/repo",
			Message:     "new auth change",
			CommittedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}))

	entries, err := eng.Timeline(ctx, "auth", now.Add(-24*time.Hour))
	require.NoError(t, err)

	for _, e := range entries {
		assert.True(t, e.Timestamp.After(now.Add(-25*time.Hour)),
			"all entries should be after the since filter")
	}
}

func TestTimeline_UsesSignalTypeAsKind(t *testing.T) {
	eng, store := newTestEngine(t)
	ctx := context.Background()

	now := time.Now().UTC()

	require.NoError(t, store.IndexCommits(ctx, "/repo", []kleio.Commit{
		{
			SHA:         "c1",
			RepoPath:    "/repo",
			Message:     "feat: add checkout flow",
			CommittedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
		},
	}))

	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:         "dec-1",
		SignalType: kleio.SignalTypeDecision,
		Content:    "decided to use Stripe for checkout payments",
		SourceType: kleio.SourceTypeManual,
		CreatedAt:  now.Add(-90 * time.Minute).Format(time.RFC3339),
	}))

	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:         "wi-1",
		SignalType: kleio.SignalTypeWorkItem,
		Content:    "need to add checkout error handling",
		SourceType: kleio.SourceTypeManual,
		CreatedAt:  now.Add(-80 * time.Minute).Format(time.RFC3339),
	}))

	entries, err := eng.Timeline(ctx, "checkout", time.Time{})
	require.NoError(t, err)

	kinds := map[string]bool{}
	for _, e := range entries {
		kinds[e.Kind] = true
	}

	assert.True(t, kinds[kleio.SignalTypeGitCommit], "should have git_commit kind")
	assert.True(t, kinds[kleio.SignalTypeDecision], "should have decision kind")
	assert.True(t, kinds[kleio.SignalTypeWorkItem], "should have work_item kind")
	assert.False(t, kinds["commit"], "should NOT have generic 'commit' kind")
	assert.False(t, kinds["event"], "should NOT have generic 'event' kind")
}

type stubExpander struct{ terms []string }

func (s *stubExpander) Expand(_ context.Context, _ string) []string { return s.terms }

func TestTimeline_AnchorExpanderWidensRecall(t *testing.T) {
	eng, store := newTestEngine(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, store.IndexCommits(ctx, "/repo", []kleio.Commit{
		{
			SHA:         "c1",
			RepoPath:    "/repo",
			Message:     "feat: add opengraph image rendering",
			CommittedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
		},
	}))

	baseline, err := eng.Timeline(ctx, "og", time.Time{})
	require.NoError(t, err)
	if len(baseline) != 0 {
		t.Fatalf("baseline expected 0 hits without expander (commit message is 'opengraph'), got %d", len(baseline))
	}

	eng.WithExpander(&stubExpander{terms: []string{"og", "opengraph"}})
	expanded, err := eng.Timeline(ctx, "og", time.Time{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(expanded), 1, "expander should surface commits matching aliased terms")
}

func TestTimelineScoped_RepoFilterRestrictsCommitsAndEvents(t *testing.T) {
	eng, store := newTestEngine(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, store.IndexCommits(ctx, "/repo-a", []kleio.Commit{
		{
			SHA:         "a1",
			RepoPath:    "/repo-a",
			RepoName:    "repo-a",
			Message:     "feat: add auth in repo-a",
			CommittedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
		},
	}))
	require.NoError(t, store.IndexCommits(ctx, "/repo-b", []kleio.Commit{
		{
			SHA:         "b1",
			RepoPath:    "/repo-b",
			RepoName:    "repo-b",
			Message:     "feat: add auth in repo-b",
			CommittedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}))
	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:         "ev-a",
		SignalType: kleio.SignalTypeDecision,
		Content:    "auth decision in repo-a",
		SourceType: kleio.SourceTypeCLI,
		RepoName:   "repo-a",
		CreatedAt:  now.Add(-90 * time.Minute).Format(time.RFC3339),
	}))
	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:         "ev-b",
		SignalType: kleio.SignalTypeDecision,
		Content:    "auth decision in repo-b",
		SourceType: kleio.SourceTypeCLI,
		RepoName:   "repo-b",
		CreatedAt:  now.Add(-80 * time.Minute).Format(time.RFC3339),
	}))

	all, err := eng.TimelineScoped(ctx, "auth", "", time.Time{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(all), 4, "all-repos scope should include both repos")

	scoped, err := eng.TimelineScoped(ctx, "auth", "repo-a", time.Time{})
	require.NoError(t, err)
	for _, e := range scoped {
		if e.SHA != "" {
			assert.NotEqual(t, "b1", e.SHA, "repo-a scope should not include repo-b commit")
		}
		if e.EventID != "" {
			assert.NotEqual(t, "ev-b", e.EventID, "repo-a scope should not include repo-b event")
		}
	}
	if len(scoped) == 0 {
		t.Fatalf("expected at least one entry under repo-a scope")
	}
}

func TestTimeline_DedupGitCommitEvents(t *testing.T) {
	eng, store := newTestEngine(t)
	ctx := context.Background()

	now := time.Now().UTC()

	require.NoError(t, store.IndexCommits(ctx, "/repo", []kleio.Commit{
		{
			SHA:         "sha1",
			RepoPath:    "/repo",
			Message:     "feat: add auth module",
			CommittedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}))

	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:         "git:sha1",
		SignalType: kleio.SignalTypeGitCommit,
		Content:    "feat: add auth module",
		SourceType: kleio.SourceTypeLocalGit,
		CreatedAt:  now.Add(-1 * time.Hour).Format(time.RFC3339),
	}))

	entries, err := eng.Timeline(ctx, "auth", time.Time{})
	require.NoError(t, err)

	commitCount := 0
	for _, e := range entries {
		if e.Kind == kleio.SignalTypeGitCommit {
			commitCount++
		}
	}
	assert.Equal(t, 1, commitCount, "same commit should not appear twice")
}
