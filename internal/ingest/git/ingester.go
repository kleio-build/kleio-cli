// Package git implements a kleio.Ingester that walks one or more git
// repositories and emits one RawSignal per commit.
//
// This package intentionally does NOT replace internal/indexer.GitIndexer.
// The indexer still owns commit/file-change/identifier persistence (those
// remain in their own tables and serve trace/explain queries). This
// ingester is the *signal* surface: every commit becomes a RawSignal
// (kind = "git_commit") that can flow through the Ingest -> Correlate ->
// Synthesize pipeline alongside plan and transcript signals.
//
// Decision: we wrap gitreader.Walk rather than re-implementing it so the
// existing noise filters, branch resolution, and file-entry diffing all
// stay in one place.
package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/entity"
	"github.com/kleio-build/kleio-cli/internal/gitreader"
)

// Ingester implements kleio.Ingester for local git repositories.
type Ingester struct {
	// RepoPaths is the explicit list of repository paths to walk. When
	// empty, RepoDiscoveryFn (or the IngestScope's RepoNames) is used.
	RepoPaths []string

	// RepoDiscoveryFn returns the repository paths to ingest given the
	// scope. The CLI can plug in CursorScope-aware discovery here so a
	// `current_repo` scope only walks the active repo, while `all_repos`
	// walks every cached repo. When nil, RepoPaths is the source of
	// truth.
	RepoDiscoveryFn func(scope kleio.IngestScope) ([]string, error)
}

func New(repoPaths ...string) *Ingester { return &Ingester{RepoPaths: repoPaths} }

func (i *Ingester) Name() string { return "git" }

// Ingest walks each discovered repository's history and emits one
// RawSignal per commit. Signals are sorted by (RepoName, Timestamp) for
// deterministic downstream correlation.
func (i *Ingester) Ingest(ctx context.Context, scope kleio.IngestScope) ([]kleio.RawSignal, error) {
	repoPaths := i.RepoPaths
	if i.RepoDiscoveryFn != nil {
		discovered, err := i.RepoDiscoveryFn(scope)
		if err != nil {
			return nil, fmt.Errorf("git ingester: discover repos: %w", err)
		}
		repoPaths = append(repoPaths, discovered...)
	}
	if len(repoPaths) == 0 {
		return nil, nil
	}

	repoPaths = uniqueAbs(repoPaths)

	var out []kleio.RawSignal
	for _, repoPath := range repoPaths {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}
		signals, err := walkRepo(repoPath, scope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "git ingest: %s: %v\n", repoPath, err)
			continue
		}
		out = append(out, signals...)
	}

	sort.SliceStable(out, func(a, b int) bool {
		if out[a].RepoName != out[b].RepoName {
			return out[a].RepoName < out[b].RepoName
		}
		return out[a].Timestamp.Before(out[b].Timestamp)
	})
	return out, nil
}

// walkRepo runs gitreader.Walk and adapts each commit into a
// kleio.RawSignal with provenance metadata sufficient for the
// correlation layer (branch, files-changed, parent SHAs, KL-N refs).
func walkRepo(repoPath string, scope kleio.IngestScope) ([]kleio.RawSignal, error) {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("abs path: %w", err)
	}
	repoName := repoNameFromPath(abs)

	walkOpts := gitreader.WalkOptions{RepoPath: abs}
	if !scope.Since.IsZero() {
		walkOpts.Since = scope.Since
	}

	commits, err := gitreader.Walk(walkOpts)
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}

	out := make([]kleio.RawSignal, 0, len(commits))
	for _, c := range commits {
		files := make([]string, 0, len(c.FileEntries))
		for _, fe := range c.FileEntries {
			files = append(files, fe.Path)
		}
		entities := entity.Extract(c.Branch+" "+c.Message, kleio.AliasSourceCommitMessage)
		for _, f := range files {
			entities = append(entities, entity.ExtractedEntity{
				Kind:       kleio.EntityKindFile,
				Value:      entity.NormalizePath(f),
				Confidence: 0.9,
				Source:     kleio.AliasSourceCommitMessage,
			})
		}
		md := map[string]any{
			"sha":                c.Hash,
			"author":             c.Author,
			"author_email":       c.Email,
			"branch":             c.Branch,
			"is_merge":           c.IsMerge,
			"files_changed":      len(files),
			"files":              files,
			"repo_path":          abs,
			"extracted_entities": entities,
		}

		out = append(out, kleio.RawSignal{
			SourceType:   kleio.SourceTypeLocalGit,
			SourceID:     fmt.Sprintf("git:%s:%s", repoName, c.Hash),
			SourceOffset: fmt.Sprintf("commit:%s", c.Hash),
			Content:      strings.TrimSpace(c.Message),
			Kind:         kleio.SignalTypeGitCommit,
			Timestamp:    c.Timestamp,
			Author:       c.Author,
			RepoName:     repoName,
			Metadata:     md,
		})
	}
	return out, nil
}

// repoNameFromPath returns the GitHub-style "owner/repo" if it can be
// derived from the configured remote (origin). Falls back to the
// directory basename when no recognizable remote exists, which matches
// the indexer's behavior so cluster_anchor_id values line up across
// ingesters.
func repoNameFromPath(abs string) string {
	repo, err := gogit.PlainOpenWithOptions(abs, &gogit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return filepath.Base(abs)
	}
	cfg, err := repo.Config()
	if err != nil {
		return filepath.Base(abs)
	}
	if origin, ok := cfg.Remotes["origin"]; ok && len(origin.URLs) > 0 {
		if name := repoNameFromRemote(origin.URLs[0]); name != "" {
			return name
		}
	}
	for _, r := range cfg.Remotes {
		if r == nil {
			continue
		}
		for _, u := range r.URLs {
			if name := repoNameFromRemote(u); name != "" {
				return name
			}
		}
	}
	return filepath.Base(abs)
}

// repoNameFromRemote pulls "owner/repo" out of a git remote URL, e.g.
//   - https://github.com/kleio-build/kleio-cli.git -> kleio-cli
//   - git@github.com:kleio-build/kleio-cli.git    -> kleio-cli
func repoNameFromRemote(url string) string {
	u := strings.TrimSuffix(url, ".git")
	if i := strings.LastIndexAny(u, "/:"); i >= 0 {
		return u[i+1:]
	}
	return ""
}

func uniqueAbs(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}
