package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/indexer"
	"github.com/spf13/cobra"
)

func NewImportCmd(getStore func() kleio.Store, getClient func() *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import external data into Kleio",
	}

	cmd.AddCommand(newImportGitCmd())
	cmd.AddCommand(newImportADRCmd(getClient))
	cmd.AddCommand(newImportCursorCmd(getStore))
	return cmd
}

func newImportGitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git [path]",
		Short: "Import git history into the local Kleio database",
		Long: `Walks the git history of the current (or given) repository and indexes
all commits, file changes, and identifiers (tickets, PRs, tags) into the
local .kleio/kleio.db. Re-running is incremental: only new commits are added.

Examples:
  kleio import git              # import current repo
  kleio import git ../other-repo`,
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
				return fmt.Errorf("import failed: %w", err)
			}

			printIndexResult(result)
			return nil
		},
	}
	return cmd
}

func newImportADRCmd(getClient func() *client.Client) *cobra.Command {
	var repoName string

	cmd := &cobra.Command{
		Use:   "adr [directory]",
		Short: "Import ADR/MADR markdown files as decision captures",
		Long: `Recursively scans a directory for .md files, parses them as Architecture
Decision Records (MADR or Nygard format), and imports them into Kleio as
decision captures.

Requires Kleio Cloud (kleio login).

Examples:
  kleio import adr ./docs/decisions/ --repo my-app
  kleio import adr ./docs/adr/ --repo my-service`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args[0]

			info, err := os.Stat(dir)
			if err != nil {
				return fmt.Errorf("cannot access %s: %w", dir, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", dir)
			}

			var files []client.ADRFileInput
			err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
					return nil
				}

				content, err := os.ReadFile(path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
					return nil
				}

				relPath, _ := filepath.Rel(filepath.Dir(dir), path)
				if relPath == "" {
					relPath = path
				}
				relPath = filepath.ToSlash(relPath)

				files = append(files, client.ADRFileInput{
					FilePath: relPath,
					Content:  string(content),
				})
				return nil
			})
			if err != nil {
				return fmt.Errorf("scanning directory: %w", err)
			}

			if len(files) == 0 {
				fmt.Println("No .md files found in", dir)
				return nil
			}

			fmt.Printf("Found %d markdown files. Importing...\n", len(files))

			result, err := getClient().ImportADRs(repoName, files)
			if err != nil {
				return fmt.Errorf("import failed: %w", err)
			}

			fmt.Printf("\nImported: %d   Skipped: %d\n\n", result.Imported, result.Skipped)
			for _, item := range result.Items {
				if item.Skipped {
					fmt.Printf("  SKIP  %s (%s)\n", item.FilePath, item.Reason)
				} else {
					fmt.Printf("  OK    %s — %s [%s]\n", item.FilePath, item.Title, item.Format)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&repoName, "repo", "", "Repository name to associate with imported ADRs")
	cmd.MarkFlagRequired("repo")
	return cmd
}
