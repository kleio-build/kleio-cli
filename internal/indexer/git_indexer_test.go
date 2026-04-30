package indexer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test User",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test User",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %s\n%s", args, err, string(out))
		}
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0o644))
	run("add", ".")
	run("commit", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "auth.go"), []byte("package auth"), 0o644))
	run("add", ".")
	run("commit", "-m", "feat: add auth module PROJ-101")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "auth_test.go"), []byte("package auth"), 0o644))
	run("add", ".")
	run("commit", "-m", "test: add auth tests (#3)")

	return dir
}

func newTestStore(t *testing.T) *localdb.Store {
	t.Helper()
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return localdb.New(db)
}

func TestGitIndexer_FullIndex(t *testing.T) {
	// Arrange
	repoDir := setupTestRepo(t)
	store := newTestStore(t)
	indexer := NewGitIndexer(store)

	// Act
	result, err := indexer.Index(context.Background(), repoDir)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, 3, result.CommitsIndexed)
	assert.Greater(t, result.FilesTracked, 0)
	assert.Greater(t, result.Identifiers, 0)
	assert.Greater(t, result.Links, 0)
	assert.False(t, result.Incremental)
	assert.Greater(t, result.Duration.Nanoseconds(), int64(0))
}

func TestGitIndexer_IncrementalIndex(t *testing.T) {
	// Arrange
	repoDir := setupTestRepo(t)
	store := newTestStore(t)
	indexer := NewGitIndexer(store)

	// First run
	_, err := indexer.Index(context.Background(), repoDir)
	require.NoError(t, err)

	// Add another commit
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test User",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test User",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "new.go"), []byte("package main"), 0o644))
	run("add", ".")
	run("commit", "-m", "feat: add new feature")

	// Act — second run
	result, err := indexer.Index(context.Background(), repoDir)
	require.NoError(t, err)

	// Assert
	assert.True(t, result.Incremental)
	assert.GreaterOrEqual(t, result.CommitsIndexed, 1)
}

func TestGitIndexer_RepoTracking(t *testing.T) {
	// Arrange
	repoDir := setupTestRepo(t)
	store := newTestStore(t)
	indexer := NewGitIndexer(store)

	// Act
	_, err := indexer.Index(context.Background(), repoDir)
	require.NoError(t, err)

	absPath, _ := filepath.Abs(repoDir)
	repo, err := store.GetRepo(context.Background(), absPath)
	require.NoError(t, err)

	// Assert
	require.NotNil(t, repo)
	assert.Equal(t, absPath, repo.Path)
	assert.NotEmpty(t, repo.LastIndexedSHA)
	assert.NotEmpty(t, repo.LastIndexedAt)
}

func TestGitIndexer_ExtractsTickets(t *testing.T) {
	// Arrange
	repoDir := setupTestRepo(t)
	store := newTestStore(t)
	indexer := NewGitIndexer(store)

	// Act
	result, err := indexer.Index(context.Background(), repoDir)
	require.NoError(t, err)

	// Assert — should have found PROJ-101 and #3
	assert.GreaterOrEqual(t, result.Identifiers, 2)
}

func TestGitIndexer_FileHistory(t *testing.T) {
	// Arrange
	repoDir := setupTestRepo(t)
	store := newTestStore(t)
	indexer := NewGitIndexer(store)

	// Act
	_, err := indexer.Index(context.Background(), repoDir)
	require.NoError(t, err)

	history, err := store.FileHistory(context.Background(), "auth.go")
	require.NoError(t, err)

	// Assert
	assert.GreaterOrEqual(t, len(history), 1)
	assert.Equal(t, "auth.go", history[0].FilePath)
}

func TestGitIndexer_QueryCommitsAfterIndex(t *testing.T) {
	// Arrange
	repoDir := setupTestRepo(t)
	store := newTestStore(t)
	indexer := NewGitIndexer(store)

	_, err := indexer.Index(context.Background(), repoDir)
	require.NoError(t, err)

	absPath, _ := filepath.Abs(repoDir)

	// Act
	commits, err := store.QueryCommits(context.Background(), kleio.CommitFilter{
		RepoPath:      absPath,
		MessageSearch: "auth",
	})
	require.NoError(t, err)

	// Assert
	assert.GreaterOrEqual(t, len(commits), 1)
}
