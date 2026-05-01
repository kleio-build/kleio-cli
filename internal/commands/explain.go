package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/ai"
	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/kleio-build/kleio-cli/internal/gitreader"
	"github.com/kleio-build/kleio-cli/internal/render"
	"github.com/spf13/cobra"
)

// NewExplainCmd creates the `kleio explain` command.
func NewExplainCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		since         string
		format        string
		output        string
		verbose       bool
		noLLM         bool
		asJSON        bool
		noInteractive bool
		allRepos      bool
	)

	cmd := &cobra.Command{
		Use:   "explain <source> <target>",
		Short: "Explain what changed between two points and why",
		Long: `Compare two git refs, branches, or tags and explain changes.

Examples:
  kleio explain HEAD~5 HEAD
  kleio explain main feature/auth-refactor
  kleio explain v1.2.0 v1.3.0
  kleio explain --format md HEAD~10 HEAD`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			source, target := args[0], args[1]
			store := getStore()

			provider := ai.AutoDetect(ai.LoadConfig())
			if noLLM {
				provider = ai.Noop{}
			}
			eng := engine.New(store, provider).WithExpander(loadAnchorExpander(provider))
			_ = noInteractive

			anchor := fmt.Sprintf("%s..%s", source, target)
			scopeRepo := ""
			if !allRepos {
				scopeRepo = currentRepoName()
			}
			entries := buildExplainEntries(store, eng, source, target, since, scopeRepo)

			if asJSON && format == "" {
				format = "json"
			}

			report := eng.BuildReport(context.Background(), anchor, "explain", entries)
			if !noLLM {
				_ = report.Enrich(context.Background(), provider)
			}
			return render.Render(os.Stdout, format, output, report, verbose)
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "Time window filter")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text, md, html, pdf, json")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write output to file")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Include raw timeline in output")
	cmd.Flags().BoolVar(&noLLM, "no-llm", false, "Skip LLM enrichment even if available")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON (shorthand for --format json)")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "Suppress prompts")
	cmd.Flags().BoolVar(&allRepos, "all-repos", false, "Include events from all indexed repositories (default: current repo only)")

	return cmd
}

func buildExplainEntries(store kleio.Store, eng *engine.Engine, source, target, since, repoName string) []engine.TimelineEntry {
	rangeCommits, err := gitreader.CommitRange(".", source, target)
	if err != nil {
		sinceTime, _ := parseSince(since)
		entries, tlErr := eng.TimelineScoped(context.Background(), "", repoName, sinceTime)
		if tlErr != nil {
			return nil
		}
		return entries
	}

	var entries []engine.TimelineEntry
	for _, rc := range rangeCommits {
		entries = append(entries, engine.TimelineEntry{
			Timestamp: rc.Timestamp,
			Kind:      kleio.SignalTypeGitCommit,
			Summary:   firstLine(rc.Message),
			SHA:       rc.Hash,
			FilePaths: rc.Files,
		})
	}

	events, _ := store.ListEvents(context.Background(), kleio.EventFilter{
		RepoName: repoName,
		Limit:    200,
	})
	for _, ev := range events {
		t, _ := time.Parse(time.RFC3339, ev.CreatedAt)
		entries = append(entries, engine.TimelineEntry{
			Timestamp: t,
			Kind:      ev.SignalType,
			Summary:   firstLine(ev.Content),
			EventID:   ev.ID,
		})
	}
	return entries
}

func extractSubsystem(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 1 {
		return "root"
	}
	return parts[0]
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

func parseSinceOrDefault(s string, defaultDays int) time.Time {
	if s != "" {
		if t, err := parseSince(s); err == nil && !t.IsZero() {
			return t
		}
	}
	return time.Now().Add(-time.Duration(defaultDays) * 24 * time.Hour)
}
