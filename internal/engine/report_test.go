package engine

import (
	"context"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildReport_Subject(t *testing.T) {
	eng, _ := newTestEngine(t)
	now := time.Now()

	entries := []TimelineEntry{
		{Timestamp: now.Add(-2 * time.Hour), Kind: kleio.SignalTypeGitCommit, Summary: "add auth", SHA: "abc123"},
		{Timestamp: now.Add(-1 * time.Hour), Kind: kleio.SignalTypeDecision, Summary: "use JWT"},
	}

	r := eng.BuildReport(context.Background(), "auth", "trace", entries)

	assert.Contains(t, r.Subject, "2 signals")
	assert.Contains(t, r.Subject, "auth")
	assert.Equal(t, "auth", r.Anchor)
	assert.Equal(t, "trace", r.Command)
	assert.False(t, r.Enriched)
}

func TestBuildReport_Decisions(t *testing.T) {
	eng, store := newTestEngine(t)
	ctx := context.Background()
	now := time.Now()

	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:             "dec-1",
		SignalType:     kleio.SignalTypeDecision,
		Content:        "Use JWT over sessions",
		SourceType:     "mcp",
		CreatedAt:      now.Add(-1 * time.Hour).Format(time.RFC3339),
		StructuredData: `{"rationale":"Stateless architecture needed"}`,
	}))

	entries := []TimelineEntry{
		{Timestamp: now.Add(-1 * time.Hour), Kind: kleio.SignalTypeDecision, Summary: "Use JWT over sessions", EventID: "dec-1"},
	}

	r := eng.BuildReport(ctx, "auth", "trace", entries)

	require.Len(t, r.Decisions, 1)
	assert.Equal(t, "Use JWT over sessions", r.Decisions[0].Content)
	assert.Equal(t, "Stateless architecture needed", r.Decisions[0].Rationale)
}

func TestBuildReport_OpenThreadsDedup(t *testing.T) {
	eng, _ := newTestEngine(t)
	now := time.Now()

	entries := []TimelineEntry{
		{Timestamp: now.Add(-3 * time.Hour), Kind: kleio.SignalTypeWorkItem, Summary: "Add error handling to checkout"},
		{Timestamp: now.Add(-2 * time.Hour), Kind: kleio.SignalTypeWorkItem, Summary: "Add error handling to checkout"},
		{Timestamp: now.Add(-1 * time.Hour), Kind: kleio.SignalTypeWorkItem, Summary: "Fix payment gateway timeout"},
	}

	r := eng.BuildReport(context.Background(), "checkout", "trace", entries)

	assert.Len(t, r.OpenThreads, 2)
	for _, thread := range r.OpenThreads {
		if thread.Content == "Add error handling to checkout" {
			assert.Equal(t, 2, thread.Occurrences)
		}
	}
}

func TestBuildReport_OpenThreadsDeferred(t *testing.T) {
	eng, _ := newTestEngine(t)
	now := time.Now()

	entries := []TimelineEntry{
		{Timestamp: now, Kind: kleio.SignalTypeWorkItem, Summary: "Defer until next sprint: refactor auth"},
		{Timestamp: now, Kind: kleio.SignalTypeWorkItem, Summary: "Out of scope for now: caching layer"},
	}

	r := eng.BuildReport(context.Background(), "auth", "trace", entries)

	for _, thread := range r.OpenThreads {
		assert.True(t, thread.Deferred, "thread %q should be deferred", thread.Content)
	}
}

func TestBuildReport_CodeChanges(t *testing.T) {
	eng, _ := newTestEngine(t)
	now := time.Now()

	entries := []TimelineEntry{
		{Timestamp: now.Add(-2 * time.Hour), Kind: kleio.SignalTypeGitCommit, Summary: "feat: auth", SHA: "abc123", FilePaths: []string{"auth.go"}},
		{Timestamp: now.Add(-1 * time.Hour), Kind: kleio.SignalTypeGitCommit, Summary: "fix: auth bug", SHA: "def456"},
		{Timestamp: now, Kind: kleio.SignalTypeGitCommit, Summary: "feat: auth again", SHA: "abc123"},
	}

	r := eng.BuildReport(context.Background(), "auth", "trace", entries)

	assert.Len(t, r.CodeChanges, 2, "duplicate SHA should be deduped")
	assert.Equal(t, "abc123", r.CodeChanges[0].SHA)
	assert.Equal(t, "def456", r.CodeChanges[1].SHA)
}

func TestBuildReport_NextSteps(t *testing.T) {
	eng, _ := newTestEngine(t)
	now := time.Now()

	entries := []TimelineEntry{
		{Timestamp: now, Kind: kleio.SignalTypeGitCommit, Summary: "init", SHA: "abc1234567890"},
	}

	r := eng.BuildReport(context.Background(), "auth", "trace", entries)

	require.GreaterOrEqual(t, len(r.NextSteps), 2)
	assert.Contains(t, r.NextSteps[0], "kleio explain")
	assert.Contains(t, r.NextSteps[1], "kleio backlog list")
}

func TestBuildReport_EvidenceQualityDupNotes(t *testing.T) {
	eng, _ := newTestEngine(t)
	now := time.Now()

	entries := []TimelineEntry{
		{Timestamp: now.Add(-2 * time.Hour), Kind: kleio.SignalTypeWorkItem, Summary: "same issue"},
		{Timestamp: now.Add(-1 * time.Hour), Kind: kleio.SignalTypeWorkItem, Summary: "same issue"},
	}

	r := eng.BuildReport(context.Background(), "test", "trace", entries)

	found := false
	for _, n := range r.EvidenceQuality.Notes {
		if assert.ObjectsAreEqual("1 work item(s) appear duplicated across re-imported transcripts.", n) {
			found = true
		}
	}
	assert.True(t, found, "should note duplicated work items")
}

func TestBuildReport_Empty(t *testing.T) {
	eng, _ := newTestEngine(t)
	r := eng.BuildReport(context.Background(), "nothing", "trace", nil)

	assert.Contains(t, r.Subject, "No signals")
	assert.Empty(t, r.Decisions)
	assert.Empty(t, r.OpenThreads)
	assert.Empty(t, r.CodeChanges)
}

func TestContentHash(t *testing.T) {
	h1 := contentHash("Add error handling")
	h2 := contentHash("  add error handling  ")
	assert.Equal(t, h1, h2, "hash should be case/whitespace insensitive")
}

func TestCosineSim(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{1, 0, 0}
	assert.InDelta(t, 1.0, cosineSim(a, b), 0.001)

	c := []float64{0, 1, 0}
	assert.InDelta(t, 0.0, cosineSim(a, c), 0.001)

	assert.Equal(t, 0.0, cosineSim(nil, nil))
	assert.Equal(t, 0.0, cosineSim([]float64{1}, []float64{1, 2}))
}
