package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/kleio-build/kleio-cli/internal/indexer"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/kleio-build/kleio-cli/internal/storeutil"
	"github.com/spf13/cobra"
)

func NewIndexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index [path]",
		Short: "Index git history into the local Kleio database",
		Long: `Walks the git history of the current (or given) repository and indexes
all commits, file changes, and identifiers (tickets, PRs, tags) into the
local .kleio/kleio.db. Re-running is incremental: only new commits are added.

Examples:
  kleio index              # index current repo
  kleio index ../other-repo`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath := "."
			if len(args) > 0 {
				repoPath = args[0]
			}

			store, err := resolveLocalDBStore()
			if err != nil {
				return fmt.Errorf("open local store: %w", err)
			}
			defer store.Close()

			idx := indexer.NewGitIndexer(store)
			result, err := idx.Index(context.Background(), repoPath)
			if err != nil {
				return fmt.Errorf("index failed: %w", err)
			}

			printIndexResult(result)
			return nil
		},
	}
	return cmd
}

func resolveLocalDBStore() (*localdb.Store, error) {
	dbPath, err := storeutil.FindDBPath()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(dbPath[:len(dbPath)-len("/kleio.db")], 0755); err != nil {
		return nil, err
	}

	db, err := localdb.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return localdb.New(db), nil
}

func printIndexResult(r *indexer.IndexResult) {
	mode := "Full"
	if r.Incremental {
		mode = "Incremental"
	}
	fmt.Printf("%s index of %s completed in %s\n", mode, r.RepoName, r.Duration.Round(100*1e6))
	fmt.Printf("  Commits: %d indexed\n", r.CommitsIndexed)
	fmt.Printf("  Files:   %d tracked\n", r.FilesTracked)
	fmt.Printf("  IDs:     %d extracted (tickets, PRs, tags)\n", r.Identifiers)
	fmt.Printf("  Links:   %d created\n", r.Links)
	if r.IsSquashHeavy {
		fmt.Println("  Note:    repository appears squash-heavy; analysis may adapt strategy")
	}
}
