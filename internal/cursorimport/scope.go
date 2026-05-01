package cursorimport

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// CursorScope controls which Cursor projects DiscoverTranscripts and friends
// will include. The default mode (current_repo) prevents one repo's .kleio
// database from being polluted with transcripts authored against unrelated
// repos.
type CursorScope struct {
	// Mode is one of: "current_repo", "workspace_file", "explicit_projects",
	// "all". Defaults to "current_repo" when unspecified.
	Mode string `yaml:"mode"`
	// WorkspaceFile is a path to a .code-workspace JSON file (used when
	// Mode == "workspace_file"). Resolved relative to the .kleio config dir.
	WorkspaceFile string `yaml:"workspace_file"`
	// ExplicitProjects is the list of cursor project slugs to include
	// (used when Mode == "explicit_projects").
	ExplicitProjects []string `yaml:"explicit_projects"`
}

// scopeConfigFile is the YAML structure read from .kleio/config.yaml when
// the user wants to override default scoping. Its presence is optional.
type scopeConfigFile struct {
	CursorScope CursorScope `yaml:"cursor_scope"`
}

// LoadCursorScope walks up from cwd looking for .kleio/config.yaml and
// returns the cursor_scope: block if present. Returns the safe default
// (Mode: "current_repo") when no config is found or the file is missing
// the cursor_scope: stanza.
func LoadCursorScope(cwd string) CursorScope {
	def := CursorScope{Mode: "current_repo"}
	dir := cwd
	for {
		candidate := filepath.Join(dir, ".kleio", "config.yaml")
		if data, err := os.ReadFile(candidate); err == nil {
			var f scopeConfigFile
			if yaml.Unmarshal(data, &f) == nil && f.CursorScope.Mode != "" {
				if f.CursorScope.WorkspaceFile != "" && !filepath.IsAbs(f.CursorScope.WorkspaceFile) {
					f.CursorScope.WorkspaceFile = filepath.Join(filepath.Dir(candidate), f.CursorScope.WorkspaceFile)
				}
				return f.CursorScope
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return def
		}
		dir = parent
	}
}

// RepoFromProjectSlug attempts to recover the absolute filesystem path that
// produced the given Cursor project slug. The Cursor convention encodes a
// directory's full path by replacing path separators with dashes (and on
// Windows, prefixing with "c-" for the drive letter).
//
// Example slugs:
//   - "c-Users-brenn-go-src-github-com-kleio-build-kleio-cli" (Windows)
//   - "-Users-bob-projects-myapp" (Unix)
//
// The encoding is ambiguous because folder names may contain literal '-'
// characters that are indistinguishable from the path separator. We resolve
// this by depth-first searching across all valid splits of the slug,
// pruning branches as soon as the partial path stops existing on disk.
//
// When a directory containing .git is found at any leaf candidate,
// RepoFromProjectSlug returns (owner, name, absPath) where owner is derived
// from the GitHub remote and name is the basename of absPath.
//
// When the slug cannot be resolved, all three return values are empty
// strings (callers should treat this as "unknown" rather than an error).
func RepoFromProjectSlug(slug string) (owner, name, absPath string) {
	if slug == "" {
		return "", "", ""
	}
	prefix, body := decodeSlugPrefix(slug)
	if prefix == "" {
		return "", "", ""
	}
	return dfsResolveSlug(prefix, body)
}

// decodeSlugPrefix returns (rootPath, remaining-slug) by stripping the
// drive-letter or leading-separator prefix. On Windows, "c-Users-..." -> ("C:\\", "Users-...").
// On Unix, "-Users-..." -> ("/", "Users-...").
func decodeSlugPrefix(slug string) (string, string) {
	if runtime.GOOS == "windows" && len(slug) >= 2 && slug[1] == '-' {
		drive := strings.ToUpper(string(slug[0]))
		return drive + ":" + string(filepath.Separator), slug[2:]
	}
	if strings.HasPrefix(slug, "-") {
		return string(filepath.Separator), strings.TrimPrefix(slug, "-")
	}
	return "", ""
}

// dfsResolveSlug walks the remaining slug DFS-style, treating each '-' as
// either a path separator, a literal hyphen, or a literal '.' inside a
// folder name. The first split that yields an existing directory
// containing .git wins.
func dfsResolveSlug(curPath, remaining string) (owner, name, absPath string) {
	dashIndices := indexAll(remaining, '-')
	for i := len(dashIndices) - 1; i >= 0; i-- {
		idx := dashIndices[i]
		head := remaining[:idx]
		tail := remaining[idx+1:]
		for _, candHead := range expandSegmentVariants(head) {
			next := filepath.Join(curPath, candHead)
			if !isDir(next) {
				continue
			}
			if owner, name, abs := dfsResolveSlug(next, tail); abs != "" {
				return owner, name, abs
			}
		}
	}
	for _, cand := range expandSegmentVariants(remaining) {
		candidate := filepath.Join(curPath, cand)
		if !isDir(candidate) {
			continue
		}
		gitPath := filepath.Join(candidate, ".git")
		info, err := os.Stat(gitPath)
		if err != nil {
			continue
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		return ownerFromGitConfig(gitPath), filepath.Base(candidate), candidate
	}
	return "", "", ""
}

// expandSegmentVariants returns every plausible folder name that the
// encoded segment could decode to. Each '-' position can be the literal
// '-' (kept) or a '.' (substituted). We cap the variant count so a path
// with many dashes doesn't explode (rare; folder names with >5 dashes are
// uncommon enough that we'd rather fail than burn cycles).
func expandSegmentVariants(s string) []string {
	dashIdxs := indexAll(s, '-')
	if len(dashIdxs) == 0 {
		return []string{s}
	}
	const cap = 32
	n := 1
	for i := 0; i < len(dashIdxs); i++ {
		n <<= 1
		if n >= cap {
			n = cap
			break
		}
	}
	out := make([]string, 0, n)
	for mask := 0; mask < n; mask++ {
		b := []byte(s)
		for j, di := range dashIdxs {
			if j >= 5 {
				break
			}
			if mask&(1<<j) != 0 {
				b[di] = '.'
			}
		}
		out = append(out, string(b))
	}
	return out
}

func indexAll(s string, c byte) []int {
	var out []int
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			out = append(out, i)
		}
	}
	return out
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// slugToPath decodes a Cursor project slug back to a filesystem path.
//
// On Windows, slugs typically begin with a single lowercase letter followed
// by '-' representing the drive (e.g. "c-Users-..." -> "C:/Users/..."). On
// Unix, slugs typically begin with a leading '-' representing the root
// (e.g. "-Users-bob-..." -> "/Users/bob/...").
//
// Folder names containing literal '-' characters cannot be losslessly
// decoded; in that case the caller's progressive walk-up to find .git
// recovers a valid repo root because parent directories rarely contain '-'.
func slugToPath(slug string) string {
	if slug == "" {
		return ""
	}
	if runtime.GOOS == "windows" && len(slug) >= 2 && slug[1] == '-' {
		drive := strings.ToUpper(string(slug[0]))
		rest := strings.ReplaceAll(slug[2:], "-", string(filepath.Separator))
		return drive + ":" + string(filepath.Separator) + rest
	}
	if !strings.HasPrefix(slug, "-") {
		// Slug doesn't match either convention; treat as relative.
		return strings.ReplaceAll(slug, "-", string(filepath.Separator))
	}
	return string(filepath.Separator) + strings.ReplaceAll(strings.TrimPrefix(slug, "-"), "-", string(filepath.Separator))
}

// ownerFromGitConfig parses .git/config and returns the GitHub owner segment
// of the [remote "origin"] URL, when one exists.
func ownerFromGitConfig(gitPath string) string {
	configPath := filepath.Join(gitPath, "config")
	// Handle .git as a file (worktree pointer)
	if info, err := os.Stat(gitPath); err == nil && !info.IsDir() {
		if data, err := os.ReadFile(gitPath); err == nil {
			line := strings.TrimSpace(string(data))
			if after, ok := strings.CutPrefix(line, "gitdir: "); ok {
				target := strings.TrimSpace(after)
				configPath = filepath.Join(target, "config")
			}
		}
	}

	f, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	inOrigin := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inOrigin = strings.EqualFold(line, `[remote "origin"]`)
			continue
		}
		if inOrigin {
			key, val, ok := strings.Cut(line, "=")
			if !ok || strings.TrimSpace(key) != "url" {
				continue
			}
			return ownerFromRemoteURL(strings.TrimSpace(val))
		}
	}
	return ""
}

// ownerFromRemoteURL strips host and trailing repo path to leave only the
// GitHub owner. Returns "" for non-GitHub URLs.
func ownerFromRemoteURL(rawURL string) string {
	for _, prefix := range []string{"git@github.com:", "https://github.com/", "http://github.com/", "ssh://git@github.com/"} {
		if rest, ok := strings.CutPrefix(rawURL, prefix); ok {
			parts := strings.SplitN(rest, "/", 2)
			if len(parts) > 0 {
				return parts[0]
			}
		}
	}
	return ""
}

// DiscoverTranscriptsScoped returns transcript paths that match the given
// CursorScope. Each returned entry includes its detected project slug so
// callers can derive RepoName before persisting events.
func DiscoverTranscriptsScoped(scope CursorScope, cwd string) ([]ScopedTranscript, error) {
	base := cursorProjectsDir()
	if base == "" {
		return nil, fmt.Errorf("could not determine Cursor projects directory")
	}
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return nil, nil
	}

	allow, err := buildProjectAllowlist(scope, cwd, base)
	if err != nil {
		return nil, err
	}

	var out []ScopedTranscript
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, fmt.Errorf("read cursor projects: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		if allow != nil && !allow[slug] {
			continue
		}
		owner, name, _ := RepoFromProjectSlug(slug)
		transcriptDir := filepath.Join(base, slug, "agent-transcripts")
		_ = filepath.Walk(transcriptDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if !info.IsDir() && strings.HasSuffix(info.Name(), ".jsonl") {
				out = append(out, ScopedTranscript{
					Path:        path,
					ProjectSlug: slug,
					RepoOwner:   owner,
					RepoName:    name,
				})
			}
			return nil
		})
	}
	return out, nil
}

// ScopedTranscript carries provenance the legacy DiscoverTranscripts loses:
// which Cursor project slug a transcript came from and the resolved git
// owner/repo so events can be tagged with RepoName at ingest time.
type ScopedTranscript struct {
	Path        string
	ProjectSlug string
	RepoOwner   string
	RepoName    string
}

// buildProjectAllowlist returns the set of cursor project slugs permitted by
// the scope. Returns nil to mean "no filter" (Mode == "all").
func buildProjectAllowlist(scope CursorScope, cwd, projectsBase string) (map[string]bool, error) {
	switch scope.Mode {
	case "", "current_repo":
		allow := map[string]bool{}
		for _, slug := range projectSlugsForCwd(cwd, projectsBase) {
			allow[slug] = true
		}
		return allow, nil
	case "explicit_projects":
		allow := make(map[string]bool, len(scope.ExplicitProjects))
		for _, s := range scope.ExplicitProjects {
			allow[s] = true
		}
		return allow, nil
	case "workspace_file":
		slugs, err := projectSlugsFromWorkspace(scope.WorkspaceFile, projectsBase)
		if err != nil {
			return nil, err
		}
		allow := make(map[string]bool, len(slugs))
		for _, s := range slugs {
			allow[s] = true
		}
		return allow, nil
	case "all":
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown cursor_scope.mode %q", scope.Mode)
	}
}

// projectSlugsForCwd returns every Cursor project slug whose decoded path
// is either the current repo root or an ancestor of the current repo root.
// We accept ancestors so workspace-level slugs (e.g. user opens Cursor at
// "$HOME/dev" containing many repos) still feed transcripts to repos
// underneath, while strictly excluding sibling repos.
//
// Note: we cannot disambiguate which sibling repo a workspace-level slug's
// transcripts belong to without reading the transcripts themselves; that
// finer-grained tagging is the job of later pipeline phases. For Phase 0
// we settle for "include every workspace whose root contains the cwd repo"
// which is a meaningful improvement over the previous all-or-nothing
// behaviour without admitting unrelated projects.
func projectSlugsForCwd(cwd, projectsBase string) []string {
	repoRoot := findRepoRoot(cwd)
	if repoRoot == "" {
		return nil
	}
	repoRoot = filepath.Clean(repoRoot)
	repoRootLower := strings.ToLower(repoRoot)

	var slugs []string
	seen := map[string]bool{}

	// Direct slug encoding (fast path).
	if slug := pathToSlug(repoRoot); slug != "" {
		if _, err := os.Stat(filepath.Join(projectsBase, slug)); err == nil {
			slugs = append(slugs, slug)
			seen[slug] = true
		}
	}

	entries, err := os.ReadDir(projectsBase)
	if err != nil {
		return slugs
	}
	for _, entry := range entries {
		if !entry.IsDir() || seen[entry.Name()] {
			continue
		}
		// Try as repo root (returns abs only when .git found at some prefix).
		_, _, abs := RepoFromProjectSlug(entry.Name())
		if abs == "" {
			// Not a git repo. Try as workspace root: decode the slug to a
			// directory path and accept it when that directory is an
			// ancestor of repoRoot.
			abs = decodeSlugToExistingDir(entry.Name())
			if abs == "" {
				continue
			}
		}
		abs = filepath.Clean(abs)
		absLower := strings.ToLower(abs)
		if absLower == repoRootLower || strings.HasPrefix(repoRootLower, absLower+string(filepath.Separator)) {
			slugs = append(slugs, entry.Name())
			seen[entry.Name()] = true
		}
	}
	return slugs
}

// decodeSlugToExistingDir DFS-walks the slug treating each '-' as either a
// path separator or a literal hyphen, returning the deepest decoded path
// that exists on disk as a directory (or "" if no such path exists). This
// is used when the slug points at a workspace folder rather than a git
// repo (no .git anywhere along the candidate path).
func decodeSlugToExistingDir(slug string) string {
	prefix, body := decodeSlugPrefix(slug)
	if prefix == "" {
		return ""
	}
	return dfsDir(prefix, body)
}

func dfsDir(curPath, remaining string) string {
	idxs := indexAll(remaining, '-')
	for i := len(idxs) - 1; i >= 0; i-- {
		head := remaining[:idxs[i]]
		tail := remaining[idxs[i]+1:]
		for _, candHead := range expandSegmentVariants(head) {
			next := filepath.Join(curPath, candHead)
			if !isDir(next) {
				continue
			}
			if abs := dfsDir(next, tail); abs != "" {
				return abs
			}
		}
	}
	for _, cand := range expandSegmentVariants(remaining) {
		candidate := filepath.Join(curPath, cand)
		if isDir(candidate) {
			return candidate
		}
	}
	return ""
}

func findRepoRoot(dir string) string {
	for {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			_ = info
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// pathToSlug encodes a filesystem path into Cursor's slug format. On
// Windows, drive C:\Users\foo becomes "c-Users-foo"; on Unix, /Users/foo
// becomes "-Users-foo". Both path separators and literal '.' characters
// inside folder names collapse to '-' (matching Cursor's own behaviour:
// "github.com" -> "github-com").
func pathToSlug(p string) string {
	if p == "" {
		return ""
	}
	rep := strings.NewReplacer(string(filepath.Separator), "-", ".", "-")
	if runtime.GOOS == "windows" {
		if len(p) >= 3 && p[1] == ':' {
			drive := strings.ToLower(string(p[0]))
			return drive + "-" + rep.Replace(p[3:])
		}
		return rep.Replace(p)
	}
	return strings.ReplaceAll(p, "/", "-")
}

// projectSlugsFromWorkspace parses a VS Code .code-workspace file (JSON,
// permits comments) and returns the cursor project slugs corresponding to
// each folder entry that resolves to a real git repo on disk.
func projectSlugsFromWorkspace(workspaceFile, projectsBase string) ([]string, error) {
	if workspaceFile == "" {
		return nil, fmt.Errorf("cursor_scope.workspace_file is required when mode=workspace_file")
	}
	data, err := os.ReadFile(workspaceFile)
	if err != nil {
		return nil, fmt.Errorf("read workspace file %s: %w", workspaceFile, err)
	}
	folders, err := parseWorkspaceFolders(data)
	if err != nil {
		return nil, fmt.Errorf("parse workspace folders in %s: %w", workspaceFile, err)
	}

	wsDir := filepath.Dir(workspaceFile)
	var slugs []string
	for _, folder := range folders {
		abs := folder
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(wsDir, abs)
		}
		root := findRepoRoot(abs)
		if root == "" {
			continue
		}
		slug := pathToSlug(root)
		if slug == "" {
			continue
		}
		slugs = append(slugs, slug)
	}
	return slugs, nil
}
