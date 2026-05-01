package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/kleio-build/kleio-cli/internal/ai"
	"github.com/kleio-build/kleio-cli/internal/correlate/embed"
	"github.com/kleio-build/kleio-cli/internal/correlate/filepath"
	"github.com/kleio-build/kleio-cli/internal/correlate/idreference"
	"github.com/kleio-build/kleio-cli/internal/correlate/search"
	"github.com/kleio-build/kleio-cli/internal/correlate/timewindow"
	kleio "github.com/kleio-build/kleio-core"
)

// newDevCorrelateCmd registers `kleio dev correlate --dry-run` for
// end-to-end correlator verification. Runs every ingester, then every
// correlator, then prints cluster counts and a sample of the largest
// clusters per correlator.
//
// EmbedCorrelator is wired only when ai.AutoDetect returns an
// available Provider; otherwise SearchCorrelator handles semantic
// correlation via Store.Search (FTS5 locally, embeddings in cloud).
// This is the same auto-promotion the production pipeline uses, so
// the verification surface matches the production surface.
func newDevCorrelateCmd() *cobra.Command {
	var (
		dryRun        bool
		allRepos      bool
		jsonOut       bool
		sampleSize    int
		windowMinutes int
	)
	cmd := &cobra.Command{
		Use:   "correlate",
		Short: "Run all ingesters + correlators and dump cluster samples (dry-run)",
		Long: `Runs the full Ingest -> Correlate pass against the current scope and
prints per-correlator cluster counts, the top-N largest clusters per
correlator, and the auto-promoted EmbedCorrelator status.

This command never persists to the local DB; it's intended for the
Phase 3 verification checklist and ongoing pipeline regression checks.
Use 'kleio ingest' (Phase 5.2) for the persisted production path.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun {
				return fmt.Errorf("only --dry-run is supported here; use 'kleio ingest' for persistence")
			}
			d := resolveDiscovery(allRepos)
			ctx := cmd.Context()

			scope := kleio.IngestScope{}
			ingesters := []kleio.Ingester{
				d.PlanIngester(),
				d.TranscriptIngester(),
				d.GitIngester(),
			}
			var allSignals []kleio.RawSignal
			for _, ing := range ingesters {
				signals, err := ing.Ingest(ctx, scope)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  %s ingest error: %v\n", ing.Name(), err)
					continue
				}
				fmt.Fprintf(os.Stderr, "[ingest] %s: %d signals\n", ing.Name(), len(signals))
				allSignals = append(allSignals, signals...)
			}
			fmt.Fprintf(os.Stderr, "[ingest] total: %d signals\n\n", len(allSignals))

			correlators := buildCorrelators(time.Duration(windowMinutes)*time.Minute, allRepos)
			for _, cor := range correlators {
				clusters, err := cor.Correlate(ctx, allSignals)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  %s correlate error: %v\n", cor.Name(), err)
					continue
				}
				printCorrelatorSummary(cor.Name(), clusters, sampleSize, jsonOut)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Run without persisting (only mode currently supported)")
	cmd.Flags().BoolVar(&allRepos, "all-repos", false, "Override cursor_scope to 'all'")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit cluster JSON to stdout")
	cmd.Flags().IntVar(&sampleSize, "sample", 3, "Number of largest clusters per correlator to display")
	cmd.Flags().IntVar(&windowMinutes, "window", 15, "TimeWindowCorrelator window size in minutes")
	return cmd
}

// buildCorrelators assembles the canonical correlator stack with
// EmbedCorrelator auto-promoted via ai.AutoDetect. When an LLM is
// available, EmbedCorrelator REPLACES SearchCorrelator (per the user's
// directive in Phase 1.4 -- "embed_replaces_when_available"). When no
// LLM is available, SearchCorrelator handles semantic correlation via
// Store.Search.
func buildCorrelators(window time.Duration, _ bool) []kleio.Correlator {
	provider := ai.AutoDetect(nil)
	correlators := []kleio.Correlator{
		timewindow.New(window),
		idreference.New(),
		filepath.New(),
	}
	if provider != nil && provider.Available() {
		fmt.Fprintf(os.Stderr, "[promote] embed correlator active (ai.Provider available)\n\n")
		correlators = append(correlators, embed.New(provider))
	} else {
		fmt.Fprintf(os.Stderr, "[promote] search correlator active (no LLM available)\n\n")
		// Store-backed search correlator wires the same store the
		// pipeline persists to. We pass nil here for the dev path
		// because no DB is open in dry-run mode; the production
		// pipeline (Phase 5.1) wires it correctly.
		correlators = append(correlators, search.New(nil))
	}
	return correlators
}

func printCorrelatorSummary(name string, clusters []kleio.Cluster, sample int, jsonOut bool) {
	fmt.Fprintf(os.Stderr, "[%s] %d clusters\n", name, len(clusters))
	if len(clusters) == 0 {
		return
	}
	sort.Slice(clusters, func(i, j int) bool {
		return len(clusters[i].Members) > len(clusters[j].Members)
	})
	limit := sample
	if limit > len(clusters) {
		limit = len(clusters)
	}
	for i := 0; i < limit; i++ {
		c := clusters[i]
		anchorContent := ""
		for _, m := range c.Members {
			if signalKey(m) == c.AnchorID {
				anchorContent = truncateString(m.Content, 80)
				break
			}
		}
		fmt.Fprintf(os.Stderr, "  cluster #%d: anchor=%s (%s) members=%d conf=%.2f\n",
			i+1, truncateString(c.AnchorID, 40), c.AnchorType, len(c.Members), c.Confidence)
		if anchorContent != "" {
			fmt.Fprintf(os.Stderr, "    \"%s\"\n", anchorContent)
		}
		if len(c.Provenance) > 0 {
			fmt.Fprintf(os.Stderr, "    via=%s\n", c.Provenance[0])
		}
	}
	fmt.Fprintln(os.Stderr)
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		for _, c := range clusters {
			_ = enc.Encode(c)
		}
	}
}

func signalKey(s kleio.RawSignal) string {
	if s.SourceID != "" {
		return s.SourceID
	}
	return s.SourceType + ":" + s.SourceOffset
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
