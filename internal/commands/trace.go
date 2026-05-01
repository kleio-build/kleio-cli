package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/ai"
	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/kleio-build/kleio-cli/internal/render"

	"github.com/spf13/cobra"
)

// NewTraceCmd creates the `kleio trace` command.
func NewTraceCmd(getStore func() kleio.Store) *cobra.Command {
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
		Use:   "trace <anchor>",
		Short: "Trace how a file, feature, or topic evolved over time",
		Long: `Reconstruct the chronological evolution of a topic, file, or feature.

Examples:
  kleio trace src/auth/service.go
  kleio trace "checkout flow"
  kleio trace --since 7d "JWT refresh"
  kleio trace --format pdf --output report.pdf "auth"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			anchor := strings.Join(args, " ")
			isFiletrace := false
			if len(args) >= 2 && args[0] == "file" {
				anchor = strings.Join(args[1:], " ")
				isFiletrace = true
			}
			store := getStore()

			provider := ai.AutoDetect(ai.LoadConfig())
			if noLLM {
				provider = ai.Noop{}
			}
			eng := engine.New(store, provider).WithExpander(loadAnchorExpander(provider))

			sinceTime, _ := parseSince(since)

			scopeRepo := ""
			if !allRepos {
				scopeRepo = currentRepoName()
			}

			var entries []engine.TimelineEntry
			var err error
			if isFiletrace {
				entries, err = eng.FileTimelineScoped(context.Background(), anchor, scopeRepo, sinceTime)
			} else if engine.IsTicketAnchor(anchor) {
				entries, err = eng.EntityTimelineScoped(context.Background(), anchor, scopeRepo, sinceTime)
			} else {
				entries, err = eng.TimelineScoped(context.Background(), anchor, scopeRepo, sinceTime)
			}
			if err != nil {
				return fmt.Errorf("trace failed: %w", err)
			}

			if len(entries) == 0 {
				if !noInteractive && isInteractive() {
					entries = runTraceRefinement(os.Stdin, os.Stderr, store, anchor, sinceTime)
				}
				if len(entries) == 0 {
					fmt.Fprintf(os.Stderr, "No results found for %q. Try broadening your search.\n", anchor)
					os.Exit(1)
				}
			}

			if asJSON && format == "" {
				format = "json"
			}

			report := eng.BuildReport(context.Background(), anchor, "trace", entries)
			if !noLLM {
				_ = report.Enrich(context.Background(), provider)
			}
			return render.Render(os.Stdout, format, output, report, verbose)
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "Time window filter (e.g. 24h, 7d, 3m)")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text, md, html, pdf, json")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write output to file (required for pdf/html if you want a specific path)")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Include raw timeline in output")
	cmd.Flags().BoolVar(&noLLM, "no-llm", false, "Skip LLM enrichment even if available")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON (shorthand for --format json)")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "Suppress prompts")
	cmd.Flags().BoolVar(&allRepos, "all-repos", false, "Include signals from all indexed repositories (default: current repo only)")

	return cmd
}

func kindIcon(kind string) string {
	switch kind {
	case kleio.SignalTypeGitCommit:
		return "[commit]"
	case kleio.SignalTypeDecision:
		return "[decision]"
	case kleio.SignalTypeWorkItem:
		return "[work_item]"
	case kleio.SignalTypeCheckpoint:
		return "[checkpoint]"
	default:
		return "[" + kind + "]"
	}
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
