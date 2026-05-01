// Package commands hosts the user-facing CLI commands. NewIngestCmd
// implements `kleio ingest`, the unified pipeline entrypoint that
// replaces the old `kleio import cursor` and adds plan + git ingestion.
//
// Compatibility: `kleio import cursor` still works (registered by
// NewImportCmd) and runs the legacy transcript-only path. New users
// should prefer `kleio ingest`.
package commands

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kleio-build/kleio-cli/internal/ai"
	"github.com/kleio-build/kleio-cli/internal/ingest/discovery"
	"github.com/kleio-build/kleio-cli/internal/pipeline"
	kleio "github.com/kleio-build/kleio-core"
)

// NewIngestCmd returns the canonical `kleio ingest` command. The
// getStore closure mirrors the pattern used by every other command:
// the Store is resolved lazily so non-DB commands stay fast.
func NewIngestCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		dryRun        bool
		allRepos      bool
		windowMinutes int
		sourceFlag    string
		sinceArg      string
		reimport      bool
		noLLM         bool
	)
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Run the unified ingest -> correlate -> synthesize pipeline",
		Long: `Discovers and ingests RawSignals from every supported source (plans,
transcripts, git commits), correlates them into clusters, and synthesises
clusters into Kleio captures.

By default ingest is scoped to the current repository (cursor_scope.mode =
current_repo). Use --all-repos to widen scope, or configure cursor_scope:
in .kleio/config.yaml.

When ai.AutoDetect finds an LLM provider (e.g. ollama running on
localhost:11434), the pipeline auto-promotes:
  - SearchCorrelator -> EmbedCorrelator (embeddings replace FTS5)
  - LLMSynthesizer is added as a final summary pass

Examples:
  kleio ingest --dry-run                  # see what would be ingested
  kleio ingest                            # persist signals to .kleio/kleio.db
  kleio ingest --source plan --dry-run    # only run the plan ingester
  kleio ingest --all-repos                # widen scope to all known repos
  kleio ingest --since 2026-04-01T00:00:00Z
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getwd: %w", err)
			}
			d := discovery.Resolve(cwd, allRepos)

			var provider ai.Provider
			if !noLLM {
				provider = ai.AutoDetect(nil)
			}

			cfg := pipeline.Config{
				Discovery:  d,
				Provider:   provider,
				TimeWindow: time.Duration(windowMinutes) * time.Minute,
			}
			if !dryRun {
				cfg.Store = getStore()
			}
			if sourceFlag != "" && sourceFlag != "all" {
				cfg.EnabledIngesters = parseCSVFlags(sourceFlag)
			}

			scope := kleio.IngestScope{AllRepos: allRepos}
			if sinceArg != "" {
				t, err := parseSince(sinceArg)
				if err != nil {
					return fmt.Errorf("parse --since: %w", err)
				}
				scope.Since = t
			}

			fmt.Fprintln(os.Stderr, "kleio ingest: starting pipeline...")
			fmt.Fprintf(os.Stderr, "  scope_mode=%s all_repos=%v llm=%v\n",
				d.CursorScope.Mode, allRepos, provider != nil && provider.Available())

			if reimport && !dryRun {
				store := cfg.Store
				if err := wipeSynthesizedEvents(cmd.Context(), store); err != nil {
					return fmt.Errorf("--reimport wipe failed: %w", err)
				}
			}

			if dryRun {
				return runDryRun(cmd.Context(), cfg, scope)
			}
			return runPersisted(cmd.Context(), cfg, scope)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Compute everything but don't persist Events or Links")
	cmd.Flags().BoolVar(&allRepos, "all-repos", false, "Override cursor_scope.mode to 'all'")
	cmd.Flags().IntVar(&windowMinutes, "window", 15, "TimeWindowCorrelator window minutes")
	cmd.Flags().StringVar(&sourceFlag, "source", "all", "Limit ingestion to specific sources (comma-separated: plan,transcript,git)")
	cmd.Flags().StringVar(&sinceArg, "since", "", "Ignore artifacts older than this (e.g. 30d, 7d, 24h, 2026-04-01)")
	cmd.Flags().BoolVar(&reimport, "reimport", false, "Delete existing synthesized events before re-running ingest")
	cmd.Flags().BoolVar(&noLLM, "no-llm", false, "Disable LLM-powered EmbedCorrelator and LLMSynthesizer")
	return cmd
}

func runDryRun(ctx context.Context, cfg pipeline.Config, scope kleio.IngestScope) error {
	p := pipeline.Build(cfg)

	var allSignals []kleio.RawSignal
	for _, ing := range p.Ingesters {
		signals, err := ing.Ingest(ctx, scope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s ingest error: %v\n", ing.Name(), err)
			continue
		}
		fmt.Fprintf(os.Stderr, "  ingest %s: %d signals\n", ing.Name(), len(signals))
		allSignals = append(allSignals, signals...)
	}
	fmt.Fprintf(os.Stderr, "  ingest total: %d\n", len(allSignals))

	var allClusters []kleio.Cluster
	for _, cor := range p.Correlators {
		clusters, err := cor.Correlate(ctx, allSignals)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s correlate error: %v\n", cor.Name(), err)
			continue
		}
		fmt.Fprintf(os.Stderr, "  correlate %s: %d clusters\n", cor.Name(), len(clusters))
		allClusters = append(allClusters, clusters...)
	}
	fmt.Fprintf(os.Stderr, "  correlate total: %d\n", len(allClusters))

	dedupe := map[string]bool{}
	bySynth := map[string]int{}
	for _, syn := range p.Synthesizers {
		count := 0
		for _, c := range allClusters {
			evs, _ := syn.Synthesize(ctx, c)
			for _, e := range evs {
				if e.ID == "" || dedupe[e.ID] {
					continue
				}
				dedupe[e.ID] = true
				count++
			}
		}
		bySynth[syn.Name()] = count
	}
	keys := make([]string, 0, len(bySynth))
	for k := range bySynth {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(os.Stderr, "  synthesize %s: %d events (would persist)\n", k, bySynth[k])
	}
	fmt.Fprintln(os.Stderr, "kleio ingest: dry-run complete (no writes)")
	return nil
}

func runPersisted(ctx context.Context, cfg pipeline.Config, scope kleio.IngestScope) error {
	p := pipeline.Build(cfg)
	report, err := p.Run(ctx, scope)
	if err != nil {
		return fmt.Errorf("pipeline run: %w", err)
	}
	fmt.Fprintf(os.Stderr, "kleio ingest: complete in %s\n", report.Duration)
	for _, k := range sortedKeys(report.SignalsByIngester) {
		fmt.Fprintf(os.Stderr, "  ingest %s: %d signals\n", k, report.SignalsByIngester[k])
	}
	for _, k := range sortedKeys(report.ClustersByCorrelator) {
		fmt.Fprintf(os.Stderr, "  correlate %s: %d clusters\n", k, report.ClustersByCorrelator[k])
	}
	for _, k := range sortedKeys(report.EventsBySynthesizer) {
		fmt.Fprintf(os.Stderr, "  synthesize %s: %d events\n", k, report.EventsBySynthesizer[k])
	}
	fmt.Fprintf(os.Stderr, "  links created: %d\n", report.LinksCreated)
	if len(report.Errors) > 0 {
		fmt.Fprintln(os.Stderr, "  WARNINGS:")
		for _, e := range report.Errors {
			fmt.Fprintf(os.Stderr, "    %s\n", e)
		}
	}
	return nil
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func parseCSVFlags(val string) map[string]bool {
	m := make(map[string]bool)
	for _, s := range strings.Split(val, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			m[s] = true
		}
	}
	return m
}

// wipeSynthesizedEvents removes all pipeline-synthesized events so that
// a subsequent ingest re-creates them. This is the `kleio ingest --reimport`
// counterpart to `kleio import cursor --reimport`. Fail-fast on error.
func wipeSynthesizedEvents(ctx context.Context, store kleio.Store) error {
	type bulkDeleter interface {
		DeleteEventsBySourceType(ctx context.Context, sourceType string) (int, error)
	}
	d, ok := store.(bulkDeleter)
	if !ok {
		return fmt.Errorf("store does not support DeleteEventsBySourceType (need localdb backend)")
	}
	sourceTypes := []string{
		"cursor_plan", "cursor_transcript", "git_commit", "llm_summary",
	}
	total := 0
	for _, st := range sourceTypes {
		n, err := d.DeleteEventsBySourceType(ctx, st)
		if err != nil {
			return fmt.Errorf("wipe %s: %w", st, err)
		}
		total += n
	}
	fmt.Fprintf(os.Stderr, "--reimport: deleted %d existing synthesized event(s).\n", total)
	return nil
}
