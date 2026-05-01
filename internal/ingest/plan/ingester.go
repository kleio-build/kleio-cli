package plan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	kleio "github.com/kleio-build/kleio-core"
)

// Ingester implements kleio.Ingester for .cursor/plans/*.plan.md files.
//
// Discovery: plans live next to source code as `.cursor/plans/`. The
// ingester walks up from each candidate root (provided via IngestScope or
// the working directory) until it finds a `.cursor/plans/` directory.
// When IngestScope.AllRepos is set, every workspace below the discovery
// root is scanned.
//
// Repo attribution: in a multi-repo workspace, all plans live in a shared
// `.cursor/plans/` directory but may describe work in different repos.
// The ingester scans plan content for mentions of KnownRepos to attribute
// each plan to the correct repository. When CurrentRepo is set and the
// plan exclusively mentions other repos, it is filtered out.
type Ingester struct {
	// Roots is the explicit list of directories to discover plans from.
	// When empty, Ingest falls back to the cwd at call time.
	Roots []string

	// KnownRepos is the set of repository names from the workspace.
	// Used to attribute plans to the correct repo via content scanning.
	// Populated from workspace folder basenames by discovery.go.
	KnownRepos []string

	// CurrentRepo is the repo name of the CWD. When non-empty and
	// AllRepos is false, plans that exclusively mention other repos are
	// filtered out so running from kleio-docs doesn't ingest kleio-cli
	// plans into the kleio-docs DB.
	CurrentRepo string
}

// New constructs a PlanIngester rooted at one or more directories. Each
// root will be searched for a nearby .cursor/plans directory.
func New(roots ...string) *Ingester { return &Ingester{Roots: roots} }

func (i *Ingester) Name() string { return "plan" }

// Ingest discovers .plan.md files under the configured roots and returns
// a deterministic, ordered slice of RawSignals. Order is:
//   sorted by plan filename, then by SignalsFromPlan emission order
// so re-running on unchanged input yields byte-identical output.
func (i *Ingester) Ingest(ctx context.Context, scope kleio.IngestScope) ([]kleio.RawSignal, error) {
	roots := i.Roots
	if len(roots) == 0 {
		if cwd, err := os.Getwd(); err == nil {
			roots = []string{cwd}
		}
	}
	seenPlans := map[string]bool{}
	var planPaths []string
	for _, r := range roots {
		dir := DiscoverPlansDir(r)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !isPlanFile(e.Name()) {
				continue
			}
			full := filepath.Join(dir, e.Name())
			if seenPlans[full] {
				continue
			}
			seenPlans[full] = true
			planPaths = append(planPaths, full)
		}
	}
	sort.Strings(planPaths)

	fallbackRepo := scope.RepoName
	if fallbackRepo == "" && len(roots) > 0 {
		fallbackRepo = filepath.Base(roots[0])
	}

	var out []kleio.RawSignal
	for _, p := range planPaths {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}
		pp, err := ParseFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "plan ingest: skipping %s: %v\n", p, err)
			continue
		}
		if !scope.Since.IsZero() && pp.ModTime.Before(scope.Since) {
			continue
		}

		repoName := i.inferPlanRepo(pp, fallbackRepo)

		if !scope.AllRepos && i.CurrentRepo != "" && repoName != i.CurrentRepo {
			continue
		}

		out = append(out, SignalsFromPlan(pp, repoName)...)
	}
	return out, nil
}

// inferPlanRepo scans the plan's frontmatter name, overview, and body
// for mentions of known repo names and returns the most-referenced one.
// Falls back to fallbackRepo when no known repo is mentioned.
func (i *Ingester) inferPlanRepo(pp *ParsedPlan, fallbackRepo string) string {
	if len(i.KnownRepos) == 0 {
		return fallbackRepo
	}

	corpus := strings.ToLower(pp.Frontmatter.Name + " " + pp.Frontmatter.Overview + " " + pp.Body)
	bestRepo := ""
	bestCount := 0

	for _, repo := range i.KnownRepos {
		lower := strings.ToLower(repo)
		if lower == "" {
			continue
		}
		count := strings.Count(corpus, lower)
		if count > bestCount {
			bestCount = count
			bestRepo = repo
		}
	}

	if bestRepo != "" {
		return bestRepo
	}
	return fallbackRepo
}

func isPlanFile(name string) bool {
	return filepath.Ext(name) == ".md" && len(name) > len(".plan.md") &&
		name[len(name)-len(".plan.md"):] == ".plan.md"
}

// DiscoverPlansDir walks up from start looking for a `.cursor/plans/`
// directory. Returns the absolute path when found, "" otherwise.
func DiscoverPlansDir(start string) string {
	dir := start
	for {
		candidate := filepath.Join(dir, ".cursor", "plans")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
