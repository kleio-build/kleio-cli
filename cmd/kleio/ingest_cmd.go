package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kleio-build/kleio-cli/internal/ingest/discovery"
	gitingest "github.com/kleio-build/kleio-cli/internal/ingest/git"
	planingest "github.com/kleio-build/kleio-cli/internal/ingest/plan"
	transcriptingest "github.com/kleio-build/kleio-cli/internal/ingest/transcript"
	kleio "github.com/kleio-build/kleio-core"
)

// newDevIngestCmd registers `kleio dev ingest <source> --dry-run` for
// per-ingester verification. Ingesters never write to the local DB in
// dry-run mode; they emit signal counts (and optionally raw JSON) for
// audit and golden-file regeneration.
//
// This is the pre-cursor to Task 5.2's user-facing `kleio ingest`
// command -- here we only expose the readonly dry-run path used by the
// audit protocol in Phase 2 / Phase 6.
func newDevIngestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingest [source]",
		Short: "Run a single ingester in dry-run mode and emit signal counts",
		Long: `Runs one of {plan, transcript, git, all} ingesters in dry-run mode
against the current scope (CursorScope-aware) and reports the number of
RawSignals emitted, broken down by SourceOffset prefix.

Use --json to emit the full RawSignal slice as newline-delimited JSON
(suitable for piping into jq for the per-task audit checks).
`,
	}
	cmd.AddCommand(
		newDevIngestSourceCmd("plan", runIngestPlan),
		newDevIngestSourceCmd("transcript", runIngestTranscript),
		newDevIngestSourceCmd("git", runIngestGit),
		newDevIngestSourceCmd("all", runIngestAll),
	)
	return cmd
}

func newDevIngestSourceCmd(source string, run func(ctx context.Context, allRepos bool, jsonOut bool, sinceArg string) error) *cobra.Command {
	var (
		dryRun   bool
		allRepos bool
		jsonOut  bool
		since    string
	)
	cmd := &cobra.Command{
		Use:   source,
		Short: fmt.Sprintf("Run the %s ingester (dry-run)", source),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun {
				return fmt.Errorf("only --dry-run is supported in dev mode (write path lands with `kleio ingest` in Phase 5.2)")
			}
			return run(cmd.Context(), allRepos, jsonOut, since)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Run the ingester without persisting (currently the only supported mode)")
	cmd.Flags().BoolVar(&allRepos, "all-repos", false, "Override cursor_scope.mode to 'all'")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit full RawSignals as NDJSON to stdout (counts go to stderr)")
	cmd.Flags().StringVar(&since, "since", "", "RFC3339 timestamp; ignore signals older than this")
	return cmd
}

func runIngestPlan(ctx context.Context, allRepos, jsonOut bool, sinceArg string) error {
	d := resolveDiscovery(allRepos)
	signals, err := d.PlanIngester().Ingest(ctx, ingestScope(d, sinceArg))
	if err != nil {
		return fmt.Errorf("plan ingester: %w", err)
	}
	return reportSignals("plan", signals, jsonOut)
}

func runIngestTranscript(ctx context.Context, allRepos, jsonOut bool, sinceArg string) error {
	d := resolveDiscovery(allRepos)
	signals, err := d.TranscriptIngester().Ingest(ctx, ingestScope(d, sinceArg))
	if err != nil {
		return fmt.Errorf("transcript ingester: %w", err)
	}
	return reportSignals("transcript", signals, jsonOut)
}

func runIngestGit(ctx context.Context, allRepos, jsonOut bool, sinceArg string) error {
	d := resolveDiscovery(allRepos)
	signals, err := d.GitIngester().Ingest(ctx, ingestScope(d, sinceArg))
	if err != nil {
		return fmt.Errorf("git ingester: %w", err)
	}
	return reportSignals("git", signals, jsonOut)
}

func runIngestAll(ctx context.Context, allRepos, jsonOut bool, sinceArg string) error {
	d := resolveDiscovery(allRepos)
	scope := ingestScope(d, sinceArg)

	bySource := map[string][]kleio.RawSignal{}
	ingesters := map[string]kleio.Ingester{
		"plan":       d.PlanIngester(),
		"transcript": d.TranscriptIngester(),
		"git":        d.GitIngester(),
	}
	keys := make([]string, 0, len(ingesters))
	for k := range ingesters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		signals, err := ingesters[k].Ingest(ctx, scope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s ingester error: %v\n", k, err)
			continue
		}
		bySource[k] = signals
	}

	totals := 0
	for _, k := range keys {
		s := bySource[k]
		totals += len(s)
		_ = reportSignals(k, s, jsonOut)
	}
	fmt.Fprintf(os.Stderr, "TOTAL: %d signals across %d ingesters\n", totals, len(keys))
	return nil
}

// reportSignals prints summary counts to stderr and (when jsonOut is
// set) NDJSON of every signal to stdout. Counts split by SourceOffset
// prefix so the audit checks "exactly N toolcall signals" line up.
func reportSignals(name string, signals []kleio.RawSignal, jsonOut bool) error {
	counts := map[string]int{}
	repos := map[string]int{}
	kinds := map[string]int{}
	for _, s := range signals {
		prefix := s.SourceOffset
		if i := strings.IndexAny(s.SourceOffset, ":#"); i > 0 {
			prefix = s.SourceOffset[:i]
		}
		counts[prefix]++
		repos[s.RepoName]++
		kinds[s.Kind]++
	}
	fmt.Fprintf(os.Stderr, "[%s] %d signals\n", name, len(signals))
	fmt.Fprintf(os.Stderr, "  prefixes: %s\n", sortedSummary(counts))
	fmt.Fprintf(os.Stderr, "  kinds:    %s\n", sortedSummary(kinds))
	fmt.Fprintf(os.Stderr, "  repos:    %s\n", sortedSummary(repos))

	if !jsonOut {
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	for _, s := range signals {
		if err := enc.Encode(s); err != nil {
			return err
		}
	}
	return nil
}

func sortedSummary(m map[string]int) string {
	if len(m) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if k == "" {
			k = "(unknown)"
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		actual := k
		if k == "(unknown)" {
			actual = ""
		}
		fmt.Fprintf(&b, "%s=%d", k, m[actual])
	}
	return b.String()
}

func resolveDiscovery(allRepos bool) discovery.Discovery {
	cwd, _ := os.Getwd()
	return discovery.Resolve(cwd, allRepos)
}

func ingestScope(d discovery.Discovery, sinceArg string) kleio.IngestScope {
	scope := kleio.IngestScope{}
	if sinceArg != "" {
		if t, err := time.Parse(time.RFC3339, sinceArg); err == nil {
			scope.Since = t
		}
	}
	return scope
}

// silence unused import warnings for ingester packages we only reference
// indirectly via discovery (gosec/lint runs).
var _ = gitingest.New
var _ = planingest.New
var _ = transcriptingest.New
