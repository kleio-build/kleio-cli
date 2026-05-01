package commands

import (
	"context"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStoreForCommands(t *testing.T) *localdb.Store {
	t.Helper()
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return localdb.New(db)
}

func TestExtractKeywords(t *testing.T) {
	kw := extractKeywords("checkout form returns 500")
	assert.Contains(t, kw, "checkout")
	assert.Contains(t, kw, "form")
	assert.Contains(t, kw, "returns")
	assert.Contains(t, kw, "500")
	assert.NotContains(t, kw, "the")
}

func TestExtractKeywords_Empty(t *testing.T) {
	kw := extractKeywords("")
	assert.Nil(t, kw)
}

func TestScoreCommitForIncident_KeywordMatch(t *testing.T) {
	c := kleio.Commit{
		SHA:          "abc123",
		Message:      "fix: checkout form validation",
		CommittedAt:  time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		FilesChanged: 3,
	}
	keywords := []string{"checkout", "form"}
	score := scoreCommitForIncident(c, keywords, nil, time.Now().Add(-24*time.Hour))
	assert.Greater(t, score, 0.3, "keyword match should boost score")
}

func TestScoreCommitForIncident_LargeChange(t *testing.T) {
	c := kleio.Commit{
		SHA:          "abc123",
		Message:      "refactor: massive cleanup",
		CommittedAt:  time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		FilesChanged: 100,
	}
	score := scoreCommitForIncident(c, nil, nil, time.Now().Add(-24*time.Hour))
	assert.Greater(t, score, 0.4, "large change should boost score")
}

func TestClassifyRisk_Revert(t *testing.T) {
	c := kleio.Commit{Message: "revert: undo broken migration"}
	assert.Equal(t, "revert", classifyRisk(c, nil, nil))
}

func TestClassifyRisk_BugFix(t *testing.T) {
	c := kleio.Commit{Message: "fix: null pointer in login", FilesChanged: 2}
	assert.Equal(t, "bug_fix_nearby", classifyRisk(c, nil, nil))
}

func TestBuildIncidentEntries(t *testing.T) {
	store := newTestStoreForCommands(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, store.IndexCommits(ctx, "/repo", []kleio.Commit{
		{
			SHA:         "abc123",
			RepoPath:    "/repo",
			Message:     "fix: checkout returns 500 on empty cart",
			CommittedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		},
		{
			SHA:         "def456",
			RepoPath:    "/repo",
			Message:     "chore: update deps",
			CommittedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
		},
	}))

	eng := engine.New(store, nil)
	entries, err := buildIncidentEntries(ctx, eng, store,
		"checkout returns 500", nil, now.Add(-24*time.Hour), "")
	require.NoError(t, err)
	assert.Greater(t, len(entries), 0)
}

func TestParseSinceOrDefault(t *testing.T) {
	result := parseSinceOrDefault("", 7)
	expected := time.Now().Add(-7 * 24 * time.Hour)
	assert.InDelta(t, expected.Unix(), result.Unix(), 5)
}
