package discovery

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kleio-build/kleio-cli/internal/cursorimport"
	kleio "github.com/kleio-build/kleio-core"
)

// scaffold builds a temp directory tree with a fake repo (containing
// .git/), a .cursor/plans/ folder with one plan file, and a
// .code-workspace pointer when requested.
func scaffold(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	repo := filepath.Join(root, "myrepo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	plansDir := filepath.Join(repo, ".cursor", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	plan := "---\nstatus: in_progress\nslug: scaffold\n---\n# Plan\n## Out of Scope\n- defer x"
	if err := os.WriteFile(filepath.Join(plansDir, "scaffold.plan.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestResolve_DefaultModeIsCurrentRepo(t *testing.T) {
	root := scaffold(t)
	d := Resolve(filepath.Join(root, "myrepo"), false)
	if d.CursorScope.Mode != "current_repo" {
		t.Errorf("mode=%q want current_repo", d.CursorScope.Mode)
	}
	if d.AllRepos {
		t.Error("AllRepos=true want false")
	}
}

func TestResolve_AllReposOverrides(t *testing.T) {
	root := scaffold(t)
	d := Resolve(filepath.Join(root, "myrepo"), true)
	if d.CursorScope.Mode != "all" {
		t.Errorf("mode=%q want all (overridden)", d.CursorScope.Mode)
	}
	if !d.AllRepos {
		t.Error("AllRepos=false want true")
	}
}

func TestPlanRoots_DefaultsToCWD(t *testing.T) {
	root := scaffold(t)
	repo := filepath.Join(root, "myrepo")
	d := Resolve(repo, false)
	roots := d.PlanRoots()
	if len(roots) != 1 {
		t.Fatalf("want 1 plan root, got %d: %v", len(roots), roots)
	}
	if !strings.EqualFold(filepath.Clean(roots[0]), filepath.Clean(repo)) {
		t.Errorf("root[0]=%q want %q", roots[0], repo)
	}
}

func TestPlanIngester_DiscoversWalkUpFromCWD(t *testing.T) {
	root := scaffold(t)
	repo := filepath.Join(root, "myrepo")
	subdir := filepath.Join(repo, "internal", "deep")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	d := Resolve(subdir, false)
	signals, err := d.PlanIngester().Ingest(context.Background(), kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) == 0 {
		t.Fatalf("want >=1 signal from walk-up plan, got 0")
	}
}

func TestGitDiscovery_CurrentRepoFindsClosestGitRoot(t *testing.T) {
	root := scaffold(t)
	repo := filepath.Join(root, "myrepo")
	d := Resolve(repo, false)
	repos, err := d.GitDiscoveryFn()(kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("want 1 repo, got %d: %v", len(repos), repos)
	}
	if !strings.EqualFold(filepath.Clean(repos[0]), filepath.Clean(repo)) {
		t.Errorf("repo[0]=%q want %q", repos[0], repo)
	}
}

func TestGitDiscovery_WorkspaceFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("workspace file path resolution differs on windows")
	}
	root := scaffold(t)
	repoB := filepath.Join(root, "myrepoB")
	if err := os.MkdirAll(filepath.Join(repoB, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	wsFile := filepath.Join(root, "scaffold.code-workspace")
	wsContents := `{"folders":[{"path":"myrepo"},{"path":"myrepoB"}]}`
	if err := os.WriteFile(wsFile, []byte(wsContents), 0o644); err != nil {
		t.Fatal(err)
	}
	d := Discovery{
		CWD: root,
		CursorScope: cursorimport.CursorScope{
			Mode:          "workspace_file",
			WorkspaceFile: wsFile,
		},
	}
	repos, err := d.GitDiscoveryFn()(kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("want 2 repos from workspace, got %d: %v", len(repos), repos)
	}
}

func TestGitDiscovery_ExplicitProjectsMode(t *testing.T) {
	d := Discovery{
		CWD: t.TempDir(),
		CursorScope: cursorimport.CursorScope{
			Mode:             "explicit_projects",
			ExplicitProjects: []string{"nonexistent-slug-xyz"},
		},
	}
	repos, err := d.GitDiscoveryFn()(kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Errorf("want 0 (slug doesn't decode), got %d: %v", len(repos), repos)
	}
}

func TestGitDiscovery_UnknownModeReturnsError(t *testing.T) {
	d := Discovery{CursorScope: cursorimport.CursorScope{Mode: "garbage"}}
	if _, err := d.GitDiscoveryFn()(kleio.IngestScope{}); err == nil {
		t.Error("want error for unknown mode, got nil")
	}
}

func TestUniqueRoots_Dedupes(t *testing.T) {
	dir := t.TempDir()
	out := uniqueRoots([]string{dir, dir, filepath.Join(dir, "..", filepath.Base(dir))})
	if len(out) != 1 {
		t.Fatalf("want 1 unique entry, got %d: %v", len(out), out)
	}
}

func TestPlanRoots_WorkspaceFileAddsFolders(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("workspace file path resolution differs on windows")
	}
	root := scaffold(t)
	repoB := filepath.Join(root, "myrepoB")
	if err := os.MkdirAll(filepath.Join(repoB, ".cursor", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	wsFile := filepath.Join(root, "scaffold.code-workspace")
	if err := os.WriteFile(wsFile, []byte(`{"folders":[{"path":"myrepo"},{"path":"myrepoB"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	d := Discovery{
		CWD: filepath.Join(root, "myrepo"),
		CursorScope: cursorimport.CursorScope{
			Mode:          "workspace_file",
			WorkspaceFile: wsFile,
		},
	}
	roots := d.PlanRoots()
	if len(roots) < 3 {
		// CWD + myrepo (== CWD, deduped) + myrepoB
		t.Fatalf("want >=2 unique plan roots, got %d: %v", len(roots), roots)
	}
}

// Smoke against a real subprocess so we know `git init` is available
// for the workspace_file test on systems where it's installed.
func TestGitInitAvailable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping workspace tests that require it")
	}
}
