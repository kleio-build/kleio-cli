package commands

import (
	"encoding/json"
	"testing"

	"github.com/kleio-build/kleio-cli/internal/gitreader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildScanCaptureInput_UsesCliSourceType(t *testing.T) {
	// Arrange
	task := gitreader.Task{Summary: "feat: add auth"}

	// Act
	input := buildScanCaptureInput(task, "feat: add auth", "my-repo")

	// Assert
	assert.Equal(t, "cli", input.SourceType)
}

func TestBuildScanCaptureInput_IncludesCommitSHA(t *testing.T) {
	// Arrange
	task := gitreader.Task{
		Commits: []gitreader.Commit{{Hash: "abc12345def"}},
		Summary: "feat: add auth",
	}

	// Act
	input := buildScanCaptureInput(task, "feat: add auth", "")
	var sd map[string]interface{}
	require.NotNil(t, input.StructuredData)
	require.NoError(t, json.Unmarshal([]byte(*input.StructuredData), &sd))

	// Assert
	assert.Equal(t, "local_git", sd["ingest_source"])
	assert.Equal(t, "abc12345def", sd["commit_sha"])
}

func TestBuildScanCaptureInput_MultipleCommitSHAs(t *testing.T) {
	// Arrange
	task := gitreader.Task{
		Commits: []gitreader.Commit{
			{Hash: "aaa111"},
			{Hash: "bbb222"},
		},
		Summary: "feat: multiple changes",
	}

	// Act
	input := buildScanCaptureInput(task, "feat: multiple changes", "")
	var sd map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(*input.StructuredData), &sd))

	// Assert
	assert.Nil(t, sd["commit_sha"], "single-sha key should not be present for multiple commits")
	shas, ok := sd["commit_shas"].([]interface{})
	require.True(t, ok, "commit_shas should be a []interface{}")
	assert.Equal(t, []interface{}{"aaa111", "bbb222"}, shas)
}

func TestBuildScanCaptureInput_IncludesBranch(t *testing.T) {
	// Arrange
	task := gitreader.Task{
		Commits: []gitreader.Commit{{Hash: "abc123"}},
		Branch:  "feature/cool",
		Summary: "feat: cool",
	}

	// Act
	input := buildScanCaptureInput(task, "feat: cool", "")
	var sd map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(*input.StructuredData), &sd))

	// Assert
	assert.Equal(t, "feature/cool", sd["branch"])
}

func TestBuildScanCaptureInput_SetsRepoName(t *testing.T) {
	// Arrange
	task := gitreader.Task{Summary: "fix: typo"}

	// Act
	input := buildScanCaptureInput(task, "fix: typo", "my-repo")

	// Assert
	require.NotNil(t, input.RepoName)
	assert.Equal(t, "my-repo", *input.RepoName)
}

func TestBuildScanCaptureInput_OmitsRepoNameWhenEmpty(t *testing.T) {
	// Arrange
	task := gitreader.Task{Summary: "fix: typo"}

	// Act
	input := buildScanCaptureInput(task, "fix: typo", "")

	// Assert
	assert.Nil(t, input.RepoName)
}

func TestBuildScanCaptureInput_FreeformContextContainsProvenance(t *testing.T) {
	// Arrange
	task := gitreader.Task{
		Commits: []gitreader.Commit{{Hash: "abc12345def67890"}},
		Branch:  "main",
		Summary: "chore: update deps",
	}

	// Act
	input := buildScanCaptureInput(task, "chore: update deps", "")

	// Assert
	require.NotNil(t, input.FreeformContext)
	assert.Contains(t, *input.FreeformContext, "Imported from local git history")
	assert.Contains(t, *input.FreeformContext, "branch: main")
	assert.Contains(t, *input.FreeformContext, "abc12345")
}

func TestBuildScanCaptureInput_SignalTypeIsWorkItem(t *testing.T) {
	// Arrange
	task := gitreader.Task{Summary: "feat: new thing"}

	// Act
	input := buildScanCaptureInput(task, "feat: new thing", "")

	// Assert
	assert.Equal(t, "work_item", input.SignalType)
}
