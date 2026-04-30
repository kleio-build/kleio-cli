package localdb_test

import (
	"context"
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFTS5_SearchFindsEventByWord(t *testing.T) {
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	s := localdb.New(db)
	ctx := context.Background()

	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeGitCommit,
		Content:    "fix: og image rendering broken on mobile",
		SourceType: kleio.SourceTypeLocalGit,
	}))

	results, err := s.Search(ctx, "og", kleio.SearchOpts{})
	require.NoError(t, err)
	found := false
	for _, r := range results {
		if r.Kind == "event" {
			found = true
			assert.Contains(t, r.Content, "og image")
		}
	}
	assert.True(t, found, "FTS5 should find 'og' in 'og image rendering broken on mobile'")
}

func TestFTS5_ListEventsWithContentSearch(t *testing.T) {
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	s := localdb.New(db)
	ctx := context.Background()

	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeDecision,
		Content:    "decided to use JWT for authentication",
		SourceType: kleio.SourceTypeManual,
	}))
	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeWorkItem,
		Content:    "need to fix the payment gateway timeout",
		SourceType: kleio.SourceTypeManual,
	}))

	events, err := s.ListEvents(ctx, kleio.EventFilter{ContentSearch: "JWT"})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Contains(t, events[0].Content, "JWT")
}

func TestFTS5_BackfillPopulatesExistingEvents(t *testing.T) {
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	s := localdb.New(db)
	ctx := context.Background()

	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeCheckpoint,
		Content:    "completed auth module with full test coverage",
		SourceType: kleio.SourceTypeCLI,
	}))

	results, err := s.Search(ctx, "auth module", kleio.SearchOpts{})
	require.NoError(t, err)
	assert.NotEmpty(t, results, "event should be searchable via FTS5")
}

func TestBackfill_CommitsToEvents(t *testing.T) {
	// Simulate a pre-existing DB: insert commits directly via SQL, then
	// re-run migration to trigger backfill.
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`INSERT INTO commits (sha, repo_path, repo_name, branch, author_name,
		author_email, message, committed_at, files_changed, insertions, deletions, is_merge, indexed_at)
		VALUES ('aaa111', '/repo', 'test-repo', 'main', 'dev', 'dev@test.com',
		'feat: add checkout flow', '2026-04-01T10:00:00Z', 3, 10, 2, 0, '2026-04-01T10:00:00Z')`)
	require.NoError(t, err)

	// Re-open to trigger migration + backfill
	err = localdb.RunMigrations(db)
	require.NoError(t, err)

	s := localdb.New(db)
	ctx := context.Background()

	events, err := s.ListEvents(ctx, kleio.EventFilter{SignalType: kleio.SignalTypeGitCommit})
	require.NoError(t, err)
	assert.Len(t, events, 1, "commit should be backfilled to events during migration")
	assert.Equal(t, "git:aaa111", events[0].ID)
	assert.Equal(t, "feat: add checkout flow", events[0].Content)
	assert.Equal(t, kleio.SourceTypeLocalGit, events[0].SourceType)
}

func TestCreateEvent_IdempotentWithDeterministicID(t *testing.T) {
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	s := localdb.New(db)
	ctx := context.Background()

	evt := &kleio.Event{
		ID:         "git:abc123",
		SignalType: kleio.SignalTypeGitCommit,
		Content:    "feat: add login",
		SourceType: kleio.SourceTypeLocalGit,
	}
	require.NoError(t, s.CreateEvent(ctx, evt))
	require.NoError(t, s.CreateEvent(ctx, evt))

	events, err := s.ListEvents(ctx, kleio.EventFilter{SignalType: kleio.SignalTypeGitCommit})
	require.NoError(t, err)
	assert.Len(t, events, 1, "duplicate insert with same ID should be ignored")
}

func TestQueryCommits_TokenizedSearch(t *testing.T) {
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	s := localdb.New(db)
	ctx := context.Background()

	require.NoError(t, s.IndexCommits(ctx, "/repo", []kleio.Commit{
		{SHA: "a1", RepoPath: "/repo", Message: "fix: og image", CommittedAt: "2026-04-01T10:00:00Z"},
		{SHA: "a2", RepoPath: "/repo", Message: "feat: add user profile", CommittedAt: "2026-04-02T10:00:00Z"},
	}))

	results, err := s.QueryCommits(ctx, kleio.CommitFilter{MessageSearch: "og image"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "a1", results[0].SHA)
}
