package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/ai"
	"github.com/kleio-build/kleio-cli/internal/ingest/discovery"
	"github.com/kleio-build/kleio-cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func newImportCursorCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		dryRun   bool
		allRepos bool
		reimport bool
	)

	cmd := &cobra.Command{
		Use:   "cursor",
		Short: "Import Cursor plans and transcripts (aliases to kleio ingest --source plan,transcript)",
		Long: `Back-compat alias for: kleio ingest --source plan,transcript

Discovers and parses Cursor plan files and agent-transcript JSONL files,
runs the ingest -> correlate -> synthesize pipeline, and persists captures.

By default only artifacts from the current repository's Cursor project
are imported (current_repo scope). Use --all-repos to widen scope.

Examples:
  kleio import cursor --dry-run
  kleio import cursor --all-repos --dry-run
  kleio import cursor --reimport`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getwd: %w", err)
			}
			d := discovery.Resolve(cwd, allRepos)
			provider := ai.AutoDetect(nil)

			cfg := pipeline.Config{
				Discovery:  d,
				Provider:   provider,
				TimeWindow: 15 * time.Minute,
				EnabledIngesters: map[string]bool{
					"plan":       true,
					"transcript": true,
				},
			}
			if !dryRun {
				cfg.Store = getStore()
			}

			scope := kleio.IngestScope{AllRepos: allRepos}

			fmt.Fprintln(os.Stderr, "kleio import cursor: running pipeline (plan + transcript)...")
			fmt.Fprintf(os.Stderr, "  scope_mode=%s all_repos=%v llm=%v\n",
				d.CursorScope.Mode, allRepos, provider != nil && provider.Available())

			if reimport && !dryRun {
				store := cfg.Store
				if err := wipeCursorPipelineEvents(cmd.Context(), store); err != nil {
					return fmt.Errorf("--reimport wipe failed: %w", err)
				}
			}

			if dryRun {
				return runDryRun(cmd.Context(), cfg, scope)
			}
			return runPersisted(cmd.Context(), cfg, scope)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be imported without persisting")
	cmd.Flags().BoolVar(&allRepos, "all-repos", false, "Override scope to import from every Cursor project on disk")
	cmd.Flags().BoolVar(&reimport, "reimport", false, "Delete existing cursor-derived events before re-running ingest")
	return cmd
}

// wipeCursorPipelineEvents removes plan + transcript synthesized events.
// Fail-fast on any error.
func wipeCursorPipelineEvents(ctx context.Context, store kleio.Store) error {
	type bulkDeleter interface {
		DeleteEventsBySourceType(ctx context.Context, sourceType string) (int, error)
	}
	d, ok := store.(bulkDeleter)
	if !ok {
		return fmt.Errorf("store does not support DeleteEventsBySourceType (need localdb backend)")
	}
	total := 0
	for _, st := range []string{"cursor_plan", "cursor_transcript", "llm_summary"} {
		n, err := d.DeleteEventsBySourceType(ctx, st)
		if err != nil {
			return fmt.Errorf("wipe %s: %w", st, err)
		}
		total += n
	}
	fmt.Fprintf(os.Stderr, "--reimport: deleted %d existing cursor-derived event(s).\n", total)
	return nil
}
