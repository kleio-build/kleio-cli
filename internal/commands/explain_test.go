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

func TestBuildExplainEntries_Integration(t *testing.T) {
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	store := localdb.New(db)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, store.IndexCommits(ctx, "/repo", []kleio.Commit{
		{SHA: "aaa111", RepoPath: "/repo", Message: "feat: initial auth module",
			CommittedAt: now.Add(-3 * time.Hour).Format(time.RFC3339), FilesChanged: 5},
		{SHA: "bbb222", RepoPath: "/repo", Message: "fix: token validation edge case",
			CommittedAt: now.Add(-2 * time.Hour).Format(time.RFC3339), FilesChanged: 2},
	}))
	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		SignalType: kleio.SignalTypeDecision,
		Content:    "Decided to use bcrypt for password hashing",
		SourceType: "mcp",
		CreatedAt:  now.Add(-2 * time.Hour).Format(time.RFC3339),
	}))

	eng := engine.New(store, nil)

	entries, err := eng.Timeline(ctx, "", time.Time{})
	require.NoError(t, err)

	report := eng.BuildReport(ctx, "HEAD~2..HEAD", "explain", entries)
	assert.NotEmpty(t, report.Subject)
}

func TestExtractSubsystem(t *testing.T) {
	assert.Equal(t, "internal", extractSubsystem("internal/auth/handler.go"))
	assert.Equal(t, "cmd", extractSubsystem("cmd/main.go"))
	assert.Equal(t, "root", extractSubsystem("go.mod"))
}

func TestAppendUnique(t *testing.T) {
	s := appendUnique([]string{"a", "b"}, "b")
	assert.Len(t, s, 2)

	s = appendUnique([]string{"a", "b"}, "c")
	assert.Len(t, s, 3)
}
