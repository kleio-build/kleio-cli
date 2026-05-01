package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/spf13/cobra"
)

// NewCorpusStatsCmd creates the `kleio dev corpus-stats` hidden command
// that prints computed CorpusStats and derived AdaptiveParams for the
// current repo. Useful for debugging "why does my report look like X?"
func NewCorpusStatsCmd(getStore func() kleio.Store) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "corpus-stats",
		Short: "Print corpus statistics and derived adaptive parameters",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := getStore()
			repoName := currentRepoName()
			ctx := context.Background()

			stats := engine.ComputeCorpusStats(ctx, store, repoName)
			params := engine.DeriveParams(stats)

			if asJSON {
				out := struct {
					RepoName string                `json:"repo_name"`
					Stats    engine.CorpusStats     `json:"stats"`
					Params   engine.AdaptiveParams  `json:"params"`
				}{
					RepoName: repoName,
					Stats:    stats,
					Params:   params,
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			fmt.Fprintf(os.Stdout, "Repo: %s\n\n", repoName)
			fmt.Fprintf(os.Stdout, "Corpus Stats:\n")
			fmt.Fprintf(os.Stdout, "  %s\n\n", stats)
			fmt.Fprintf(os.Stdout, "Derived Adaptive Params:\n")
			fmt.Fprintf(os.Stdout, "  %s\n", params)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}
