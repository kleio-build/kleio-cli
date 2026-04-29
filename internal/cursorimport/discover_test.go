package cursorimport

// Coverage contract:
//
// DiscoverTranscripts:
// - finds .jsonl files under agent-transcripts dirs
// - returns empty for non-existent base dir
// - skips non-JSONL files
//
// DiscoverTranscriptsForProject:
// - finds transcripts under a specific project slug
// - returns empty for non-existent project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestCursorDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	slug := "c-Users-test-project"
	transcriptDir := filepath.Join(home, ".cursor", "projects", slug, "agent-transcripts", "abc-123")
	require.NoError(t, os.MkdirAll(transcriptDir, 0755))

	require.NoError(t, os.WriteFile(
		filepath.Join(transcriptDir, "abc-123.jsonl"),
		[]byte(`{"role":"user","message":{"content":[{"type":"text","text":"test"}]}}`+"\n"),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(transcriptDir, "not-a-transcript.txt"),
		[]byte("ignore me"),
		0644,
	))

	return home
}

func TestDiscoverTranscripts_FindsJSONL(t *testing.T) {
	// Arrange
	setupTestCursorDir(t)

	// Act
	files, err := DiscoverTranscripts()

	// Assert
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Contains(t, files[0], "abc-123.jsonl")
}

func TestDiscoverTranscripts_EmptyForNonExistentDir(t *testing.T) {
	// Arrange
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// Act
	files, err := DiscoverTranscripts()

	// Assert
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestDiscoverTranscriptsForProject_FindsCorrectSlug(t *testing.T) {
	// Arrange
	setupTestCursorDir(t)

	// Act
	files, err := DiscoverTranscriptsForProject("c-Users-test-project")

	// Assert
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Contains(t, files[0], "abc-123.jsonl")
}

func TestDiscoverTranscriptsForProject_EmptyForWrongSlug(t *testing.T) {
	// Arrange
	setupTestCursorDir(t)

	// Act
	files, err := DiscoverTranscriptsForProject("nonexistent-project")

	// Assert
	require.NoError(t, err)
	assert.Empty(t, files)
}
