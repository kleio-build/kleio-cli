package commands

import (
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"

	"github.com/kleio-build/kleio-cli/internal/ai"
	"github.com/kleio-build/kleio-cli/internal/aliases"
	"github.com/kleio-build/kleio-cli/internal/engine"
)

// loadAnchorExpander wires the static aliases file (~/.kleio/aliases.yaml
// by default) plus optional LLM expansion onto the engine. Failures are
// swallowed and reported via stderr in --verbose mode by callers; trace/
// explain/incident must remain useful with no aliases configured.
func loadAnchorExpander(provider ai.Provider) engine.AnchorExpander {
	exp, _ := aliases.New(aliases.DefaultPath(), provider)
	return exp
}

// currentRepoName returns the repository identifier for the current working
// directory ("owner/repo" when an origin remote is configured, otherwise the
// directory basename). Returns "" when the cwd is not a git repository.
//
// trace/explain/incident default to filtering by this value so that running
// the command from inside a project surfaces only that project's signals,
// matching the import-time scope established by Phase 0 (CursorScope).
// --all-repos opts back into the full corpus.
func currentRepoName() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return repoNameFromCwd(cwd)
}

func repoNameFromCwd(cwd string) string {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return ""
	}
	repo, err := gogit.PlainOpenWithOptions(abs, &gogit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return ""
	}
	cfg, err := repo.Config()
	if err != nil {
		return filepath.Base(abs)
	}
	if origin, ok := cfg.Remotes["origin"]; ok && len(origin.URLs) > 0 {
		if name := repoNameFromRemoteURL(origin.URLs[0]); name != "" {
			return name
		}
	}
	for _, r := range cfg.Remotes {
		if r == nil {
			continue
		}
		for _, u := range r.URLs {
			if name := repoNameFromRemoteURL(u); name != "" {
				return name
			}
		}
	}
	return filepath.Base(abs)
}

func repoNameFromRemoteURL(url string) string {
	u := strings.TrimSuffix(url, ".git")
	if i := strings.LastIndexAny(u, "/:"); i >= 0 {
		return u[i+1:]
	}
	return ""
}
