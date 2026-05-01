package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/kleio-build/kleio-cli/internal/ai"
	"github.com/kleio-build/kleio-cli/internal/synthesize/llm"
	"github.com/kleio-build/kleio-cli/internal/synthesize/orphan"
	"github.com/kleio-build/kleio-cli/internal/synthesize/plancluster"
	kleio "github.com/kleio-build/kleio-core"
)

// newDevSynthesizeCmd registers `kleio dev synthesize --dry-run` for
// end-to-end synthesizer verification. Runs the full ingest +
// correlate + synthesize chain in dry-run, prints per-synthesizer
// event counts plus a sample of the resulting Events.
func newDevSynthesizeCmd() *cobra.Command {
	var (
		dryRun        bool
		allRepos      bool
		sampleSize    int
		windowMinutes int
		clusterCap    int
	)
	cmd := &cobra.Command{
		Use:   "synthesize",
		Short: "Run all ingesters + correlators + synthesizers (dry-run)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun {
				return fmt.Errorf("only --dry-run supported here; use 'kleio ingest' for persistence")
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
				allSignals = append(allSignals, signals...)
			}
			fmt.Fprintf(os.Stderr, "[ingest] total: %d signals\n\n", len(allSignals))

			correlators := buildCorrelators(time.Duration(windowMinutes)*time.Minute, allRepos)
			var allClusters []kleio.Cluster
			for _, cor := range correlators {
				clusters, err := cor.Correlate(ctx, allSignals)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  %s correlate error: %v\n", cor.Name(), err)
					continue
				}
				fmt.Fprintf(os.Stderr, "[correlate] %s: %d clusters\n", cor.Name(), len(clusters))
				allClusters = append(allClusters, clusters...)
			}
			fmt.Fprintf(os.Stderr, "[correlate] total: %d clusters\n\n", len(allClusters))

			if clusterCap > 0 && len(allClusters) > clusterCap {
				sort.Slice(allClusters, func(i, j int) bool {
					return len(allClusters[i].Members) > len(allClusters[j].Members)
				})
				allClusters = allClusters[:clusterCap]
				fmt.Fprintf(os.Stderr, "[synthesize] capped to top %d clusters\n", clusterCap)
			}

			provider := ai.AutoDetect(nil)
			synthesizers := []kleio.Synthesizer{
				plancluster.New(),
				orphan.New(),
			}
			if provider != nil && provider.Available() {
				fmt.Fprintf(os.Stderr, "[promote] llm synthesizer active\n")
				synthesizers = append(synthesizers, llm.New(provider))
			} else {
				fmt.Fprintf(os.Stderr, "[promote] llm synthesizer skipped (no provider)\n")
			}
			fmt.Fprintln(os.Stderr)

			eventsBySynth := map[string][]kleio.Event{}
			for _, syn := range synthesizers {
				seen := map[string]bool{}
				for _, c := range allClusters {
					evs, err := syn.Synthesize(ctx, c)
					if err != nil {
						continue
					}
					for _, e := range evs {
						if e.ID == "" || seen[e.ID] {
							continue
						}
						seen[e.ID] = true
						eventsBySynth[syn.Name()] = append(eventsBySynth[syn.Name()], e)
					}
				}
			}

			synthKeys := make([]string, 0, len(eventsBySynth))
			for k := range eventsBySynth {
				synthKeys = append(synthKeys, k)
			}
			sort.Strings(synthKeys)
			for _, k := range synthKeys {
				printSynthSummary(k, eventsBySynth[k], sampleSize)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Run without persisting (only mode supported)")
	cmd.Flags().BoolVar(&allRepos, "all-repos", false, "Override cursor_scope to 'all'")
	cmd.Flags().IntVar(&sampleSize, "sample", 5, "Number of events per synthesizer to display")
	cmd.Flags().IntVar(&windowMinutes, "window", 15, "TimeWindowCorrelator window minutes")
	cmd.Flags().IntVar(&clusterCap, "cluster-cap", 50, "Cap on clusters fed to synthesizers (0=unbounded; protects ollama)")
	return cmd
}

func printSynthSummary(name string, events []kleio.Event, sample int) {
	fmt.Fprintf(os.Stderr, "[%s] %d events\n", name, len(events))
	bySignal := map[string]int{}
	byRepo := map[string]int{}
	for _, e := range events {
		bySignal[e.SignalType]++
		byRepo[e.RepoName]++
	}
	fmt.Fprintf(os.Stderr, "  signals: %s\n", sortedCounts(bySignal))
	fmt.Fprintf(os.Stderr, "  repos:   %s\n", sortedCounts(byRepo))
	if sample > len(events) {
		sample = len(events)
	}
	for i := 0; i < sample; i++ {
		fmt.Fprintf(os.Stderr, "  - [%s] %s\n", events[i].SignalType, truncateString(events[i].Content, 100))
	}
	fmt.Fprintln(os.Stderr)
}

func sortedCounts(m map[string]int) string {
	if len(m) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		actual := k
		if actual == "" {
			actual = "(unknown)"
		}
		out += fmt.Sprintf("%s=%d", actual, m[k])
	}
	return out
}
