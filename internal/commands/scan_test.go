package commands

import (
	"encoding/json"
	"testing"

	"github.com/kleio-build/kleio-cli/internal/gitreader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildScanEvent_UsesCliSourceType(t *testing.T) {
	task := gitreader.Task{Summary: "feat: add auth"}
	evt := buildScanEvent(task, "feat: add auth", "my-repo")
	assert.Equal(t, "cli", evt.SourceType)
}

func TestBuildScanEvent_IncludesCommitSHA(t *testing.T) {
	task := gitreader.Task{
		Commits: []gitreader.Commit{{Hash: "abc12345def"}},
		Summary: "feat: add auth",
	}

	evt := buildScanEvent(task, "feat: add auth", "")
	var sd map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(evt.StructuredData), &sd))

	assert.Equal(t, "local_git", sd["ingest_source"])
	assert.Equal(t, "abc12345def", sd["commit_sha"])
}

func TestBuildScanEvent_MultipleCommitSHAs(t *testing.T) {
	task := gitreader.Task{
		Commits: []gitreader.Commit{
			{Hash: "aaa111"},
			{Hash: "bbb222"},
		},
		Summary: "feat: multiple changes",
	}

	evt := buildScanEvent(task, "feat: multiple changes", "")
	var sd map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(evt.StructuredData), &sd))

	assert.Nil(t, sd["commit_sha"], "single-sha key should not be present for multiple commits")
	shas, ok := sd["commit_shas"].([]interface{})
	require.True(t, ok, "commit_shas should be a []interface{}")
	assert.Equal(t, []interface{}{"aaa111", "bbb222"}, shas)
}

func TestBuildScanEvent_IncludesBranch(t *testing.T) {
	task := gitreader.Task{
		Commits: []gitreader.Commit{{Hash: "abc123"}},
		Branch:  "feature/cool",
		Summary: "feat: cool",
	}

	evt := buildScanEvent(task, "feat: cool", "")
	var sd map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(evt.StructuredData), &sd))

	assert.Equal(t, "feature/cool", sd["branch"])
}

func TestBuildScanEvent_SetsRepoName(t *testing.T) {
	task := gitreader.Task{Summary: "fix: typo"}
	evt := buildScanEvent(task, "fix: typo", "my-repo")
	assert.Equal(t, "my-repo", evt.RepoName)
}

func TestBuildScanEvent_OmitsRepoNameWhenEmpty(t *testing.T) {
	task := gitreader.Task{Summary: "fix: typo"}
	evt := buildScanEvent(task, "fix: typo", "")
	assert.Equal(t, "", evt.RepoName)
}

func TestBuildScanEvent_FreeformContextContainsProvenance(t *testing.T) {
	task := gitreader.Task{
		Commits: []gitreader.Commit{{Hash: "abc12345def67890"}},
		Branch:  "main",
		Summary: "chore: update deps",
	}

	evt := buildScanEvent(task, "chore: update deps", "")
	assert.Contains(t, evt.FreeformContext, "Imported from local git history")
	assert.Contains(t, evt.FreeformContext, "branch: main")
	assert.Contains(t, evt.FreeformContext, "abc12345")
}

func TestBuildScanEvent_SignalTypeIsWorkItem(t *testing.T) {
	task := gitreader.Task{Summary: "feat: new thing"}
	evt := buildScanEvent(task, "feat: new thing", "")
	assert.Equal(t, "work_item", evt.SignalType)
}
