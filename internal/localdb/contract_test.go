package localdb_test

import (
	"context"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openStore(t *testing.T) kleio.Store {
	t.Helper()
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	s := localdb.New(db)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestContract_Mode(t *testing.T) {
	s := openStore(t)
	assert.Equal(t, kleio.StoreModeLocal, s.Mode())
}

func TestContract_EventCRUD(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	ev := &kleio.Event{
		SignalType: kleio.SignalTypeDecision,
		Content:    "Use PostgreSQL for the main database",
		SourceType: "mcp",
	}
	require.NoError(t, s.CreateEvent(ctx, ev))
	assert.NotEmpty(t, ev.ID)
	assert.NotEmpty(t, ev.CreatedAt)

	got, err := s.GetEvent(ctx, ev.ID)
	require.NoError(t, err)
	assert.Equal(t, ev.ID, got.ID)
	assert.Equal(t, kleio.SignalTypeDecision, got.SignalType)
	assert.Equal(t, "Use PostgreSQL for the main database", got.Content)

	list, err := s.ListEvents(ctx, kleio.EventFilter{SignalType: kleio.SignalTypeDecision})
	require.NoError(t, err)
	assert.Len(t, list, 1)

	emptyList, err := s.ListEvents(ctx, kleio.EventFilter{SignalType: "nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, emptyList)
}

func TestContract_BacklogItemCRUD(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	item := &kleio.BacklogItem{
		Title:   "Add rate limiting",
		Summary: "API endpoints need rate limiting",
	}
	require.NoError(t, s.CreateBacklogItem(ctx, item))
	assert.NotEmpty(t, item.ID)

	got, err := s.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Add rate limiting", got.Title)
	assert.Equal(t, kleio.StatusOpen, got.Status)

	require.NoError(t, s.UpdateBacklogItem(ctx, item.ID, &kleio.BacklogItem{
		Status: kleio.StatusDone,
	}))

	updated, err := s.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, kleio.StatusDone, updated.Status)

	list, err := s.ListBacklogItems(ctx, kleio.BacklogFilter{Status: kleio.StatusDone})
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestContract_CommitIndex(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	commits := []kleio.Commit{
		{SHA: "aaa111", RepoPath: "/repo", Message: "feat: add auth",
			CommittedAt: now.Add(-2 * time.Hour).Format(time.RFC3339), FilesChanged: 3},
		{SHA: "bbb222", RepoPath: "/repo", Message: "fix: token expiry",
			CommittedAt: now.Add(-1 * time.Hour).Format(time.RFC3339), FilesChanged: 1},
	}
	require.NoError(t, s.IndexCommits(ctx, "/repo", commits))

	result, err := s.QueryCommits(ctx, kleio.CommitFilter{RepoPath: "/repo"})
	require.NoError(t, err)
	assert.Len(t, result, 2)

	result, err = s.QueryCommits(ctx, kleio.CommitFilter{MessageSearch: "auth"})
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "aaa111", result[0].SHA)
}

func TestContract_CommitIndex_Idempotent(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	commits := []kleio.Commit{
		{SHA: "aaa111", RepoPath: "/repo", Message: "feat: add auth",
			CommittedAt: time.Now().UTC().Format(time.RFC3339)},
	}
	require.NoError(t, s.IndexCommits(ctx, "/repo", commits))
	require.NoError(t, s.IndexCommits(ctx, "/repo", commits))

	result, err := s.QueryCommits(ctx, kleio.CommitFilter{RepoPath: "/repo"})
	require.NoError(t, err)
	assert.Len(t, result, 1)
}

func TestContract_Links(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	link := &kleio.Link{
		SourceID: "commit:aaa111",
		TargetID: "ticket:PROJ-42",
		LinkType: "references",
	}
	require.NoError(t, s.CreateLink(ctx, link))
	assert.NotEmpty(t, link.ID)

	links, err := s.QueryLinks(ctx, kleio.LinkFilter{SourceID: "commit:aaa111"})
	require.NoError(t, err)
	assert.Len(t, links, 1)
	assert.Equal(t, "references", links[0].LinkType)
}

func TestContract_LinkDedup(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		require.NoError(t, s.CreateLink(ctx, &kleio.Link{
			SourceID: "commit:aaa111",
			TargetID: "ticket:PROJ-42",
			LinkType: "references",
		}))
	}

	links, err := s.QueryLinks(ctx, kleio.LinkFilter{SourceID: "commit:aaa111"})
	require.NoError(t, err)
	assert.Len(t, links, 1, "duplicate links should be deduplicated")
}

func TestContract_FileTracking(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	require.NoError(t, s.IndexCommits(ctx, "/repo", []kleio.Commit{
		{SHA: "aaa111", RepoPath: "/repo", Message: "add file",
			CommittedAt: time.Now().UTC().Format(time.RFC3339)},
	}))

	require.NoError(t, s.TrackFileChange(ctx, &kleio.FileChange{
		CommitSHA:  "aaa111",
		FilePath:   "src/auth.go",
		ChangeType: kleio.ChangeTypeAdded,
	}))

	history, err := s.FileHistory(ctx, "src/auth.go")
	require.NoError(t, err)
	assert.Len(t, history, 1)
	assert.Equal(t, kleio.ChangeTypeAdded, history[0].ChangeType)
}

func TestContract_Search(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeDecision,
		Content:    "Decided to use Redis for caching",
		SourceType: "mcp",
	}))
	require.NoError(t, s.IndexCommits(ctx, "/repo", []kleio.Commit{
		{SHA: "aaa111", RepoPath: "/repo", Message: "feat: add Redis cache layer",
			CommittedAt: time.Now().UTC().Format(time.RFC3339)},
	}))

	results, err := s.Search(ctx, "Redis", kleio.SearchOpts{Limit: 10})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 2, "should find both event and commit")

	kinds := map[string]bool{}
	for _, r := range results {
		kinds[r.Kind] = true
	}
	assert.True(t, kinds["event"])
	assert.True(t, kinds["commit"])
}

func TestContract_SearchEmpty(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	results, err := s.Search(ctx, "nonexistent-query-xyz", kleio.SearchOpts{Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, results)
}
