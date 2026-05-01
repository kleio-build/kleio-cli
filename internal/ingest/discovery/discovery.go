// Package discovery centralises the "what to ingest" decision so every
// pipeline runner agrees on which plan files, transcript files, and git
// repositories belong to the current scope.
//
// Discovery rules:
//
//  1. Plans are found by walking up from cwd looking for .cursor/plans/.
//     If `cursor_scope.workspace_file` is set, every folder in the
//     workspace also contributes its own .cursor/plans/ directory.
//
//  2. Transcripts are scoped via cursorimport.CursorScope. Mode
//     "current_repo" (default) restricts to the active repo's slugs;
//     "workspace_file" restricts to the slugs of folders in the
//     workspace; "explicit_projects" uses the user's exact list; "all"
//     accepts every Cursor project.
//
//  3. Git repositories are derived from the same scope: current_repo
//     walks up to the .git root only; workspace_file walks each folder
//     in the workspace; "all" intentionally returns nothing because
//     ingesting every repo on the disk is rarely what the user wants
//     (and would duplicate signals).
package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kleio-build/kleio-cli/internal/cursorimport"
	gitingest "github.com/kleio-build/kleio-cli/internal/ingest/git"
	planingest "github.com/kleio-build/kleio-cli/internal/ingest/plan"
	transcriptingest "github.com/kleio-build/kleio-cli/internal/ingest/transcript"
	kleio "github.com/kleio-build/kleio-core"
)

// Discovery captures the resolved scope for a single ingest run. Each
// ingester receives a slice of paths (plans, repos) or transcripts via
// the discovery hooks below.
type Discovery struct {
	CWD         string
	CursorScope cursorimport.CursorScope
	// AllRepos overrides the cursor_scope.mode at the CLI flag level.
	// Set true by `--all-repos`. When true, transcripts widen to all
	// known Cursor projects and git ingestion uses the full set of
	// known repos.
	AllRepos bool
}

// Resolve constructs a Discovery from the user's cwd, optionally
// overlaying the --all-repos CLI flag.
func Resolve(cwd string, allRepos bool) Discovery {
	scope := cursorimport.LoadCursorScope(cwd)
	if allRepos {
		scope.Mode = "all"
	}
	return Discovery{CWD: cwd, CursorScope: scope, AllRepos: allRepos}
}

// PlanRoots returns the directories the PlanIngester should search for
// .cursor/plans/. Discovery walks up from the CWD to the closest
// .cursor/plans, and (when in workspace_file mode) every folder in the
// workspace file. Any directory that does not actually contain a
// .cursor/plans/ subtree is omitted by the ingester itself.
func (d Discovery) PlanRoots() []string {
	roots := []string{d.CWD}

	if d.CursorScope.Mode == "workspace_file" && d.CursorScope.WorkspaceFile != "" {
		folders, err := workspaceFolders(d.CursorScope.WorkspaceFile)
		if err == nil {
			for _, f := range folders {
				if abs := absoluteWorkspaceFolder(d.CursorScope.WorkspaceFile, f); abs != "" {
					roots = append(roots, abs)
				}
			}
		}
	}
	return uniqueRoots(roots)
}

// TranscriptDiscoveryFn returns the closure used to plug Discovery into
// transcript.Ingester.DiscoverTranscriptsFn.
func (d Discovery) TranscriptDiscoveryFn() func(scope kleio.IngestScope) ([]transcriptingest.TranscriptInput, error) {
	fallbackRepo := filepath.Base(d.CWD)
	return func(_ kleio.IngestScope) ([]transcriptingest.TranscriptInput, error) {
		scoped, err := cursorimport.DiscoverTranscriptsScoped(d.CursorScope, d.CWD)
		if err != nil {
			return nil, fmt.Errorf("discover transcripts: %w", err)
		}
		out := make([]transcriptingest.TranscriptInput, 0, len(scoped))
		for _, s := range scoped {
			repo := s.RepoName
			if repo == "" {
				repo = fallbackRepo
			}
			out = append(out, transcriptingest.TranscriptInput{
				Path:     s.Path,
				RepoName: repo,
			})
		}
		return out, nil
	}
}

// GitDiscoveryFn returns the closure used to plug Discovery into
// git.Ingester.RepoDiscoveryFn. Behaviour:
//
//   - current_repo (default): the closest .git root above cwd.
//   - workspace_file: every folder in the workspace, walked up to its
//     own .git root. Folders without a git root are skipped.
//   - explicit_projects: each slug is decoded back to a path via
//     cursorimport.RepoFromProjectSlug.
//   - all: returns the union of explicit_projects-style decoding for
//     every slug under the Cursor projects dir. Heavy; opt-in only.
func (d Discovery) GitDiscoveryFn() func(scope kleio.IngestScope) ([]string, error) {
	return func(_ kleio.IngestScope) ([]string, error) {
		switch d.CursorScope.Mode {
		case "", "current_repo":
			if root := repoRoot(d.CWD); root != "" {
				return []string{root}, nil
			}
			return nil, nil
		case "workspace_file":
			folders, err := workspaceFolders(d.CursorScope.WorkspaceFile)
			if err != nil {
				return nil, err
			}
			var repos []string
			for _, f := range folders {
				abs := absoluteWorkspaceFolder(d.CursorScope.WorkspaceFile, f)
				if root := repoRoot(abs); root != "" {
					repos = append(repos, root)
				}
			}
			return uniqueRoots(repos), nil
		case "explicit_projects":
			var repos []string
			for _, slug := range d.CursorScope.ExplicitProjects {
				if _, _, abs := cursorimport.RepoFromProjectSlug(slug); abs != "" {
					repos = append(repos, abs)
				}
			}
			return uniqueRoots(repos), nil
		case "all":
			return discoverAllRepos(), nil
		default:
			return nil, fmt.Errorf("unknown cursor_scope.mode %q", d.CursorScope.Mode)
		}
	}
}

// PlanIngester returns a fully configured plan ingester rooted at
// PlanRoots(). KnownRepos and CurrentRepo are derived from workspace
// folder basenames so plan content can be attributed to the correct
// repo instead of defaulting everything to CWD.
func (d Discovery) PlanIngester() *planingest.Ingester {
	ing := planingest.New(d.PlanRoots()...)
	ing.KnownRepos = d.knownRepoNames()
	ing.CurrentRepo = filepath.Base(d.CWD)
	return ing
}

// knownRepoNames returns the basenames of all repos in the workspace.
// Used by the plan ingester for content-based repo attribution. Discovery
// checks: (1) CWD, (2) workspace file folders, (3) sibling directories
// of CWD that contain .git (covers multi-repo workspace layouts like
// kleio-build/{kleio-cli,kleio-docs,kleio-core,...}).
func (d Discovery) knownRepoNames() []string {
	var names []string
	seen := map[string]bool{}
	add := func(dir string) {
		name := filepath.Base(dir)
		if name == "" || name == "." || name == "/" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	add(d.CWD)

	if d.CursorScope.Mode == "workspace_file" && d.CursorScope.WorkspaceFile != "" {
		folders, err := workspaceFolders(d.CursorScope.WorkspaceFile)
		if err == nil {
			for _, f := range folders {
				if abs := absoluteWorkspaceFolder(d.CursorScope.WorkspaceFile, f); abs != "" {
					add(abs)
				}
			}
		}
	}

	parent := filepath.Dir(d.CWD)
	if parent != d.CWD {
		entries, err := os.ReadDir(parent)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				sibling := filepath.Join(parent, e.Name())
				gitPath := filepath.Join(sibling, ".git")
				if _, err := os.Stat(gitPath); err == nil {
					add(sibling)
				}
			}
		}
	}

	return names
}

// TranscriptIngester returns a fully configured transcript ingester
// wired to TranscriptDiscoveryFn.
func (d Discovery) TranscriptIngester() *transcriptingest.Ingester {
	ing := transcriptingest.New()
	ing.DiscoverTranscriptsFn = d.TranscriptDiscoveryFn()
	return ing
}

// GitIngester returns a fully configured git ingester wired to
// GitDiscoveryFn.
func (d Discovery) GitIngester() *gitingest.Ingester {
	ing := gitingest.New()
	ing.RepoDiscoveryFn = d.GitDiscoveryFn()
	return ing
}

// repoRoot walks up from start looking for a `.git` directory or file.
// Returns the absolute path to the directory containing .git, or ""
// when no git root exists.
func repoRoot(start string) string {
	if start == "" {
		return ""
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// uniqueRoots dedupes by absolute path, preserving input order.
func uniqueRoots(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, p := range in {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		key := strings.ToLower(abs)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, abs)
	}
	return out
}

// workspaceFolders shells out to cursorimport.parseWorkspaceFolders via
// a small unmarshal so this package doesn't need to duplicate the
// JSON-with-comments parser. The cursorimport package already exposes
// LoadCursorScope() but not the folder parser, so we re-import the file
// with a thin wrapper.
func workspaceFolders(workspaceFile string) ([]string, error) {
	data, err := os.ReadFile(workspaceFile)
	if err != nil {
		return nil, fmt.Errorf("read workspace file %s: %w", workspaceFile, err)
	}
	folders, err := cursorimport.ParseWorkspaceFolders(data)
	if err != nil {
		return nil, fmt.Errorf("parse workspace folders in %s: %w", workspaceFile, err)
	}
	return folders, nil
}

func absoluteWorkspaceFolder(workspaceFile, folder string) string {
	if filepath.IsAbs(folder) {
		return filepath.Clean(folder)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(workspaceFile), folder))
}

// discoverAllRepos returns every git repo path that can be decoded from
// a Cursor project slug. Used only for cursor_scope.mode = "all" or the
// CLI --all-repos override. Slugs that don't decode to a repo (e.g.
// workspace-level slugs) are skipped.
func discoverAllRepos() []string {
	base := cursorimport.CursorProjectsDir()
	if base == "" {
		return nil
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var repos []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, _, abs := cursorimport.RepoFromProjectSlug(e.Name()); abs != "" {
			repos = append(repos, abs)
		}
	}
	sort.Strings(repos)
	return uniqueRoots(repos)
}
