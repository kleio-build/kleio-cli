package commands

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildExplainReport_Basic(t *testing.T) {
	store, err := localdb.OpenInMemory()
	require.NoError(t, err)
	s := localdb.New(store)
	t.Cleanup(func() { s.Close() })

	eng := engine.New(s, nil)

	now := time.Now()
	entries := []engine.TimelineEntry{
		{Timestamp: now.Add(-2 * time.Hour), Kind: kleio.SignalTypeGitCommit, Summary: "feat: add auth module",
			FilePaths: []string{"internal/auth/handler.go", "internal/auth/service.go"}},
		{Timestamp: now.Add(-1 * time.Hour), Kind: kleio.SignalTypeGitCommit, Summary: "fix: auth token expiry",
			FilePaths: []string{"internal/auth/token.go"}},
		{Timestamp: now.Add(-30 * time.Minute), Kind: kleio.SignalTypeDecision, Summary: "decision: use JWT over sessions"},
	}

	report := buildExplainReport("HEAD~3", "HEAD", entries, eng)

	assert.Equal(t, "HEAD~3", report.Source)
	assert.Equal(t, "HEAD", report.Target)
	assert.Equal(t, 2, report.Commits)
	assert.Contains(t, report.Subsystems, "internal")
	assert.Len(t, report.Decisions, 1)
	assert.Contains(t, report.Decisions[0], "decision")
}

func TestBuildExplainReport_Empty(t *testing.T) {
	store, err := localdb.OpenInMemory()
	require.NoError(t, err)
	s := localdb.New(store)
	t.Cleanup(func() { s.Close() })

	eng := engine.New(s, nil)
	report := buildExplainReport("v1.0", "v2.0", nil, eng)

	assert.Equal(t, 0, report.Commits)
	assert.Equal(t, "v1.0", report.Source)
	assert.Equal(t, "v2.0", report.Target)
}

func TestRenderExplainReport(t *testing.T) {
	report := &ExplainReport{
		Source:     "main",
		Target:     "feature/auth",
		Commits:    5,
		Subsystems: map[string]int{"internal": 3, "cmd": 2},
		Decisions:  []string{"Chose JWT over session cookies"},
		Summary:    "5 commits across 2 subsystems",
		Details: []ExplainDetail{
			{Subsystem: "internal", Files: []string{"auth.go", "handler.go"}, Summary: "2 file(s) changed"},
			{Subsystem: "cmd", Files: []string{"main.go"}, Summary: "1 file(s) changed"},
		},
	}

	tmpFile, err := os.CreateTemp("", "explain-test-*.txt")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	err = renderExplainReport(tmpFile, report)
	require.NoError(t, err)
	tmpFile.Close()

	data, _ := os.ReadFile(tmpFile.Name())
	output := string(data)

	assert.Contains(t, output, "main -> feature/auth")
	assert.Contains(t, output, "Commits: 5")
	assert.Contains(t, output, "Key Decisions")
	assert.Contains(t, output, "JWT over session")
	assert.Contains(t, output, "[internal]")
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

func TestExplainCmd_Integration(t *testing.T) {
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
		SignalType: "decision",
		Content:    "Decided to use bcrypt for password hashing",
		SourceType: "mcp",
		CreatedAt:  now.Add(-2 * time.Hour).Format(time.RFC3339),
	}))

	eng := engine.New(store, nil)

	var buf bytes.Buffer
	entries, err := eng.Timeline(ctx, "", time.Time{})
	require.NoError(t, err)

	report := buildExplainReport("HEAD~2", "HEAD", entries, eng)
	assert.Greater(t, report.Commits, 0)
	_ = buf
}
