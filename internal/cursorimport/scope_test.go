package cursorimport

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlugToPath_Roundtrip(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("slug roundtrip uses platform-native separator; covered here for Windows")
	}
	cases := []struct {
		slug, want string
	}{
		{"c-Users-brenn-go-src-github-com-kleio-build-kleio-cli", `C:\Users\brenn\go\src\github\com\kleio\build\kleio\cli`},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, slugToPath(c.slug))
	}
}

func TestPathToSlug_Roundtrip(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("slug roundtrip uses platform-native separator; covered here for Windows")
	}
	assert.Equal(t, `c-Users-brenn-go-src-github-com-kleio-build-kleio-cli`,
		pathToSlug(`C:\Users\brenn\go\src\github\com\kleio\build\kleio\cli`))
}

func TestRepoFromProjectSlug_RecoversRepoFromSyntheticGit(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "kleio-cli")
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git", "config"),
		[]byte("[remote \"origin\"]\n\turl = git@github.com:kleio-build/kleio-cli.git\n"),
		0o644))

	slug := pathToSlug(repoDir)
	require.NotEmpty(t, slug)

	owner, name, abs := RepoFromProjectSlug(slug)
	assert.Equal(t, "kleio-build", owner)
	assert.Equal(t, "kleio-cli", name)
	assert.Equal(t, repoDir, abs)
}

func TestRepoFromProjectSlug_HandlesNonGitDirectory(t *testing.T) {
	owner, name, abs := RepoFromProjectSlug("definitely-not-a-real-project-slug-xyz")
	assert.Empty(t, owner)
	assert.Empty(t, name)
	assert.Empty(t, abs)
}

func TestOwnerFromRemoteURL(t *testing.T) {
	cases := []struct {
		url, want string
	}{
		{"git@github.com:kleio-build/kleio-cli.git", "kleio-build"},
		{"https://github.com/kleio-build/kleio-cli.git", "kleio-build"},
		{"http://github.com/foo/bar", "foo"},
		{"ssh://git@github.com/owner/repo", "owner"},
		{"https://gitlab.com/foo/bar.git", ""},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			assert.Equal(t, c.want, ownerFromRemoteURL(c.url))
		})
	}
}

func TestLoadCursorScope_DefaultsToCurrentRepo(t *testing.T) {
	tmp := t.TempDir()
	got := LoadCursorScope(tmp)
	assert.Equal(t, "current_repo", got.Mode)
}

func TestLoadCursorScope_ReadsAllRepos(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".kleio"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(tmp, ".kleio", "config.yaml"),
		[]byte("cursor_scope:\n  mode: all\n"),
		0o644))

	got := LoadCursorScope(tmp)
	assert.Equal(t, "all", got.Mode)
}

func TestLoadCursorScope_ReadsExplicitProjects(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".kleio"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(tmp, ".kleio", "config.yaml"),
		[]byte("cursor_scope:\n  mode: explicit_projects\n  explicit_projects: [foo, bar]\n"),
		0o644))

	got := LoadCursorScope(tmp)
	assert.Equal(t, "explicit_projects", got.Mode)
	assert.Equal(t, []string{"foo", "bar"}, got.ExplicitProjects)
}

func TestLoadCursorScope_ResolvesRelativeWorkspaceFile(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".kleio"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(tmp, ".kleio", "config.yaml"),
		[]byte("cursor_scope:\n  mode: workspace_file\n  workspace_file: ../my.code-workspace\n"),
		0o644))

	got := LoadCursorScope(tmp)
	assert.Equal(t, "workspace_file", got.Mode)
	assert.True(t, strings.HasSuffix(got.WorkspaceFile, "my.code-workspace"))
	assert.True(t, filepath.IsAbs(got.WorkspaceFile))
}

func TestParseWorkspaceFolders_StripsLineComments(t *testing.T) {
	body := []byte(`{
  // top-level comment
  "folders": [
    {"path": "./repo-a"},
    {"path": "../repo-b"}  // trailing comment
  ]
}`)
	folders, err := parseWorkspaceFolders(body)
	require.NoError(t, err)
	assert.Equal(t, []string{"./repo-a", "../repo-b"}, folders)
}

func TestParseWorkspaceFolders_KeepsCommentMarkersInsideStrings(t *testing.T) {
	body := []byte(`{"folders": [{"path": "./has//slash"}]}`)
	folders, err := parseWorkspaceFolders(body)
	require.NoError(t, err)
	assert.Equal(t, []string{"./has//slash"}, folders)
}

func TestDiscoverTranscriptsScoped_FiltersToCurrentRepo(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("scoped discovery slug encoding is platform-specific; primary target is Windows")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	// Create two cursor projects: matching repo (kleio-cli) and
	// unrelated repo (other-app). Both have transcripts.
	cwd := filepath.Join(home, "go", "src", "github.com", "kleio-build", "kleio-cli")
	require.NoError(t, os.MkdirAll(filepath.Join(cwd, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, ".git", "config"),
		[]byte(`[remote "origin"]`+"\n\turl = git@github.com:kleio-build/kleio-cli.git\n"), 0o644))

	other := filepath.Join(home, "go", "src", "github.com", "other-org", "other-app")
	require.NoError(t, os.MkdirAll(filepath.Join(other, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(other, ".git", "config"),
		[]byte(`[remote "origin"]`+"\n\turl = git@github.com:other-org/other-app.git\n"), 0o644))

	mySlug := pathToSlug(cwd)
	otherSlug := pathToSlug(other)

	for _, slug := range []string{mySlug, otherSlug} {
		dir := filepath.Join(home, ".cursor", "projects", slug, "agent-transcripts", "abc")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "abc.jsonl"), []byte("{}"), 0o644))
	}

	scope := CursorScope{Mode: "current_repo"}
	got, err := DiscoverTranscriptsScoped(scope, cwd)
	require.NoError(t, err)

	require.Len(t, got, 1, "expected only the current repo's transcript")
	assert.Equal(t, mySlug, got[0].ProjectSlug)
	assert.Equal(t, "kleio-cli", got[0].RepoName)
	assert.Equal(t, "kleio-build", got[0].RepoOwner)

	// All-mode should pick up both.
	all, err := DiscoverTranscriptsScoped(CursorScope{Mode: "all"}, cwd)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}
