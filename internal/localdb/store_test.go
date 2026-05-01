package localdb_test

import (
	"context"
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) kleio.Store {
	t.Helper()
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return localdb.New(db)
}

func TestStore_Mode(t *testing.T) {
	s := newTestStore(t)
	assert.Equal(t, kleio.StoreModeLocal, s.Mode())
}

// --- Event CRUD ---

func TestStore_CreateAndGetEvent(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	evt := &kleio.Event{
		SignalType: kleio.SignalTypeWorkItem,
		Content:    "fix: resolve auth token refresh bug",
		SourceType: kleio.SourceTypeManual,
	}

	// Act
	err := s.CreateEvent(context.Background(), evt)
	require.NoError(t, err)
	require.NotEmpty(t, evt.ID)

	got, err := s.GetEvent(context.Background(), evt.ID)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, evt.Content, got.Content)
	assert.Equal(t, evt.SignalType, got.SignalType)
	assert.Equal(t, evt.SourceType, got.SourceType)
	assert.Equal(t, kleio.AuthorTypeHuman, got.AuthorType)
	assert.NotEmpty(t, got.CreatedAt)
	assert.False(t, got.Synced)
}

func TestStore_CreateEvent_WithAllFields(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	evt := &kleio.Event{
		SignalType:      kleio.SignalTypeDecision,
		Content:         "use JWT for auth",
		SourceType:      kleio.SourceTypeCLI,
		RepoName:        "kleio-cli",
		BranchName:      "feature/auth",
		FilePath:        "internal/auth/jwt.go",
		FreeformContext: "decided after comparing session vs JWT",
		StructuredData:  `{"key":"val"}`,
		AuthorType:      kleio.AuthorTypeAgent,
		AuthorLabel:     "cursor",
	}

	// Act
	err := s.CreateEvent(context.Background(), evt)
	require.NoError(t, err)

	got, err := s.GetEvent(context.Background(), evt.ID)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, kleio.SignalTypeDecision, got.SignalType)
	assert.Equal(t, "kleio-cli", got.RepoName)
	assert.Equal(t, "feature/auth", got.BranchName)
	assert.Equal(t, "internal/auth/jwt.go", got.FilePath)
	assert.Equal(t, "decided after comparing session vs JWT", got.FreeformContext)
	assert.Equal(t, `{"key":"val"}`, got.StructuredData)
	assert.Equal(t, kleio.AuthorTypeAgent, got.AuthorType)
	assert.Equal(t, "cursor", got.AuthorLabel)
}

func TestStore_ListEvents_FilterBySignalType(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeWorkItem, Content: "item1", SourceType: kleio.SourceTypeManual,
	}))
	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeDecision, Content: "dec1", SourceType: kleio.SourceTypeManual,
	}))
	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeWorkItem, Content: "item2", SourceType: kleio.SourceTypeManual,
	}))

	// Act
	events, err := s.ListEvents(ctx, kleio.EventFilter{SignalType: kleio.SignalTypeWorkItem})
	require.NoError(t, err)

	// Assert
	assert.Len(t, events, 2)
	for _, e := range events {
		assert.Equal(t, kleio.SignalTypeWorkItem, e.SignalType)
	}
}

func TestStore_ListEvents_FilterByRepoName(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeWorkItem, Content: "a", SourceType: kleio.SourceTypeManual, RepoName: "repo-a",
	}))
	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeWorkItem, Content: "b", SourceType: kleio.SourceTypeManual, RepoName: "repo-b",
	}))

	// Act
	events, err := s.ListEvents(ctx, kleio.EventFilter{RepoName: "repo-a"})
	require.NoError(t, err)

	// Assert
	assert.Len(t, events, 1)
	assert.Equal(t, "a", events[0].Content)
}

func TestStore_ListEvents_WithLimit(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
			SignalType: kleio.SignalTypeWorkItem, Content: "item", SourceType: kleio.SourceTypeManual,
		}))
	}

	// Act
	events, err := s.ListEvents(ctx, kleio.EventFilter{Limit: 2})
	require.NoError(t, err)

	// Assert
	assert.Len(t, events, 2)
}

func TestStore_GetEvent_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetEvent(context.Background(), "nonexistent-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- Backlog CRUD ---

func TestStore_CreateAndGetBacklogItem(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	item := &kleio.BacklogItem{
		Title:    "Add rate limiting to API",
		Summary:  "Prevent abuse of the capture endpoint",
		Category: kleio.CategoryTask,
	}

	// Act
	err := s.CreateBacklogItem(context.Background(), item)
	require.NoError(t, err)
	require.NotEmpty(t, item.ID)

	got, err := s.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, "Add rate limiting to API", got.Title)
	assert.Equal(t, "Prevent abuse of the capture endpoint", got.Summary)
	assert.Equal(t, kleio.StatusOpen, got.Status)
	assert.Equal(t, kleio.CategoryTask, got.Category)
	assert.Equal(t, kleio.UrgencyMedium, got.Urgency)
	assert.Equal(t, kleio.ImportanceMedium, got.Importance)
	assert.False(t, got.Synced)
}

func TestStore_ListBacklogItems_FilterByStatus(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.CreateBacklogItem(ctx, &kleio.BacklogItem{
		Title: "open item", Status: kleio.StatusOpen,
	}))
	require.NoError(t, s.CreateBacklogItem(ctx, &kleio.BacklogItem{
		Title: "done item", Status: kleio.StatusDone,
	}))

	// Act
	items, err := s.ListBacklogItems(ctx, kleio.BacklogFilter{Status: kleio.StatusOpen})
	require.NoError(t, err)

	// Assert
	assert.Len(t, items, 1)
	assert.Equal(t, "open item", items[0].Title)
}

func TestStore_ListBacklogItems_Search(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.CreateBacklogItem(ctx, &kleio.BacklogItem{
		Title: "fix auth bug",
	}))
	require.NoError(t, s.CreateBacklogItem(ctx, &kleio.BacklogItem{
		Title: "add rate limiter",
	}))

	// Act
	items, err := s.ListBacklogItems(ctx, kleio.BacklogFilter{Search: "auth"})
	require.NoError(t, err)

	// Assert
	assert.Len(t, items, 1)
	assert.Equal(t, "fix auth bug", items[0].Title)
}

func TestStore_UpdateBacklogItem(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	item := &kleio.BacklogItem{Title: "test item"}
	require.NoError(t, s.CreateBacklogItem(ctx, item))

	// Act
	err := s.UpdateBacklogItem(ctx, item.ID, &kleio.BacklogItem{
		Status:  kleio.StatusInProgress,
		Urgency: kleio.UrgencyHigh,
	})
	require.NoError(t, err)

	// Assert
	got, err := s.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, kleio.StatusInProgress, got.Status)
	assert.Equal(t, kleio.UrgencyHigh, got.Urgency)
}

func TestStore_UpdateBacklogItem_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateBacklogItem(context.Background(), "no-such-id", &kleio.BacklogItem{
		Status: kleio.StatusDone,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- Git Index ---

func TestStore_IndexAndQueryCommits(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	commits := []kleio.Commit{
		{SHA: "abc123", RepoPath: "/tmp/repo", Message: "feat: add login", CommittedAt: "2026-04-01T10:00:00Z", AuthorName: "dev", AuthorEmail: "dev@test.com"},
		{SHA: "def456", RepoPath: "/tmp/repo", Message: "fix: login redirect", CommittedAt: "2026-04-02T10:00:00Z", AuthorName: "dev", AuthorEmail: "dev@test.com", IsMerge: true},
	}

	// Act
	err := s.IndexCommits(ctx, "/tmp/repo", commits)
	require.NoError(t, err)

	all, err := s.QueryCommits(ctx, kleio.CommitFilter{RepoPath: "/tmp/repo"})
	require.NoError(t, err)

	// Assert
	assert.Len(t, all, 2)
	assert.Equal(t, "def456", all[0].SHA) // desc order
	assert.True(t, all[0].IsMerge)
	assert.False(t, all[1].IsMerge)
}

func TestStore_IndexCommits_Idempotent(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	commits := []kleio.Commit{
		{SHA: "abc123", RepoPath: "/tmp/repo", Message: "feat: add login", CommittedAt: "2026-04-01T10:00:00Z"},
	}

	// Act — insert twice
	require.NoError(t, s.IndexCommits(ctx, "/tmp/repo", commits))
	require.NoError(t, s.IndexCommits(ctx, "/tmp/repo", commits))

	all, err := s.QueryCommits(ctx, kleio.CommitFilter{RepoPath: "/tmp/repo"})
	require.NoError(t, err)

	// Assert — still only one row
	assert.Len(t, all, 1)
}

func TestStore_QueryCommits_FilterByMessage(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.IndexCommits(ctx, "/repo", []kleio.Commit{
		{SHA: "a1", RepoPath: "/repo", Message: "feat: add payment flow", CommittedAt: "2026-04-01T10:00:00Z"},
		{SHA: "a2", RepoPath: "/repo", Message: "fix: css in header", CommittedAt: "2026-04-02T10:00:00Z"},
	}))

	// Act
	results, err := s.QueryCommits(ctx, kleio.CommitFilter{MessageSearch: "payment"})
	require.NoError(t, err)

	// Assert
	assert.Len(t, results, 1)
	assert.Equal(t, "a1", results[0].SHA)
}

func TestStore_QueryCommits_FilterByMerge(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.IndexCommits(ctx, "/repo", []kleio.Commit{
		{SHA: "m1", RepoPath: "/repo", Message: "merge PR #1", CommittedAt: "2026-04-01T10:00:00Z", IsMerge: true},
		{SHA: "c1", RepoPath: "/repo", Message: "add file", CommittedAt: "2026-04-02T10:00:00Z"},
	}))

	// Act
	isMerge := true
	results, err := s.QueryCommits(ctx, kleio.CommitFilter{IsMerge: &isMerge})
	require.NoError(t, err)

	// Assert
	assert.Len(t, results, 1)
	assert.Equal(t, "m1", results[0].SHA)
}

// --- Links ---

func TestStore_CreateAndQueryLinks(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	link := &kleio.Link{
		SourceID: "evt-1",
		TargetID: "commit-abc",
		LinkType: kleio.LinkTypeRelatedTo,
	}

	// Act
	err := s.CreateLink(ctx, link)
	require.NoError(t, err)
	require.NotEmpty(t, link.ID)

	results, err := s.QueryLinks(ctx, kleio.LinkFilter{SourceID: "evt-1"})
	require.NoError(t, err)

	// Assert
	assert.Len(t, results, 1)
	assert.Equal(t, "commit-abc", results[0].TargetID)
	assert.Equal(t, kleio.LinkTypeRelatedTo, results[0].LinkType)
	assert.Equal(t, 1.0, results[0].Confidence)
}

func TestStore_QueryLinks_FilterByLinkType(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.CreateLink(ctx, &kleio.Link{
		SourceID: "a", TargetID: "b", LinkType: kleio.LinkTypeRelatedTo,
	}))
	require.NoError(t, s.CreateLink(ctx, &kleio.Link{
		SourceID: "a", TargetID: "c", LinkType: kleio.LinkTypeTouches,
	}))

	// Act
	results, err := s.QueryLinks(ctx, kleio.LinkFilter{LinkType: kleio.LinkTypeTouches})
	require.NoError(t, err)

	// Assert
	assert.Len(t, results, 1)
	assert.Equal(t, "c", results[0].TargetID)
}

// --- File History ---

func TestStore_TrackAndQueryFileHistory(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()

	// Need commits first (foreign key)
	require.NoError(t, s.IndexCommits(ctx, "/repo", []kleio.Commit{
		{SHA: "c1", RepoPath: "/repo", Message: "add auth", CommittedAt: "2026-04-01T10:00:00Z"},
		{SHA: "c2", RepoPath: "/repo", Message: "fix auth", CommittedAt: "2026-04-02T10:00:00Z"},
	}))

	require.NoError(t, s.TrackFileChange(ctx, &kleio.FileChange{
		CommitSHA: "c1", FilePath: "auth/service.go", ChangeType: kleio.ChangeTypeAdded,
	}))
	require.NoError(t, s.TrackFileChange(ctx, &kleio.FileChange{
		CommitSHA: "c2", FilePath: "auth/service.go", ChangeType: kleio.ChangeTypeModified,
	}))

	// Act
	history, err := s.FileHistory(ctx, "auth/service.go")
	require.NoError(t, err)

	// Assert
	assert.Len(t, history, 2)
	assert.Equal(t, "c2", history[0].CommitSHA) // desc order by committed_at
	assert.Equal(t, kleio.ChangeTypeModified, history[0].ChangeType)
}

// --- Search ---

func TestStore_Search_FindsEventsAndCommits(t *testing.T) {
	// Arrange
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeWorkItem, Content: "investigate auth token expiry", SourceType: kleio.SourceTypeManual,
	}))
	require.NoError(t, s.IndexCommits(ctx, "/repo", []kleio.Commit{
		{SHA: "x1", RepoPath: "/repo", Message: "fix: auth token refresh", CommittedAt: "2026-04-01T10:00:00Z"},
	}))

	// Act
	results, err := s.Search(ctx, "auth token", kleio.SearchOpts{})
	require.NoError(t, err)

	// Assert
	assert.GreaterOrEqual(t, len(results), 2)
	kinds := map[string]bool{}
	for _, r := range results {
		kinds[r.Kind] = true
	}
	assert.True(t, kinds["event"])
	assert.True(t, kinds["commit"])
}

// --- DeleteEventsBySourceType ---

func TestStore_DeleteEventsBySourceType_RemovesOnlyMatching(t *testing.T) {
	s := newTestStore(t).(*localdb.Store)
	ctx := context.Background()

	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeWorkItem,
		Content:    "from cursor",
		SourceType: kleio.SourceTypeCursorTranscript,
	}))
	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeWorkItem,
		Content:    "from cursor 2",
		SourceType: kleio.SourceTypeCursorTranscript,
	}))
	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeDecision,
		Content:    "manual decision",
		SourceType: kleio.SourceTypeManual,
	}))

	n, err := s.DeleteEventsBySourceType(ctx, kleio.SourceTypeCursorTranscript)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	all, err := s.ListEvents(ctx, kleio.EventFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, kleio.SourceTypeManual, all[0].SourceType)

	// FTS index should also be cleansed: a search for the deleted content
	// must return zero results (otherwise re-imports would produce ghost
	// matches).
	hits, err := s.Search(ctx, "cursor", kleio.SearchOpts{})
	require.NoError(t, err)
	for _, h := range hits {
		assert.NotEqual(t, "from cursor", h.Content)
		assert.NotEqual(t, "from cursor 2", h.Content)
	}
}

// TestStore_AcceptsPipelineLinkTypes guards Task 1.4 acceptance: every
// new pipeline LinkType constant must round-trip through CreateLink ->
// QueryLinks without schema-level rejection (link_type is freeform TEXT
// today; this test is the canary for any future CHECK constraint).
func TestStore_AcceptsPipelineLinkTypes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, lt := range []string{
		kleio.LinkTypeClusterAnchor,
		kleio.LinkTypeCorrelatedWith,
		kleio.LinkTypeDerivedFrom,
		kleio.LinkTypeParentSignal,
	} {
		require.NoError(t, s.CreateLink(ctx, &kleio.Link{
			SourceID: "src-" + lt, TargetID: "tgt-" + lt, LinkType: lt,
		}), "create link with type %s", lt)
	}

	got, err := s.QueryLinks(ctx, kleio.LinkFilter{LinkType: kleio.LinkTypeClusterAnchor})
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, kleio.LinkTypeClusterAnchor, got[0].LinkType)
}

func TestStore_DeleteEventsBySourceType_NoMatchingIsNoOp(t *testing.T) {
	s := newTestStore(t).(*localdb.Store)
	ctx := context.Background()

	require.NoError(t, s.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeDecision,
		Content:    "manual",
		SourceType: kleio.SourceTypeManual,
	}))

	n, err := s.DeleteEventsBySourceType(ctx, kleio.SourceTypeCursorTranscript)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}
