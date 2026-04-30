package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/gitreader"
	"github.com/kleio-build/kleio-cli/internal/privacy"
	"github.com/spf13/cobra"
)

func NewScanCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		since      string
		author     string
		jsonOutput bool
		noFilter   bool
		importFlag bool
		dryRun     bool
		repoName   string
	)

	makeRunE := func(view gitreader.ScanView) func(cmd *cobra.Command, args []string) error {
		return func(cmd *cobra.Command, args []string) error {
			repoPath := "."
			if len(args) > 0 {
				repoPath = args[0]
			}

			var sinceTime time.Time
			if since != "" {
				parsed, err := parseSince(since)
				if err != nil {
					return fmt.Errorf("invalid --since value: %w", err)
				}
				sinceTime = parsed
			} else {
				sinceTime = defaultSince(view)
			}

			result, err := gitreader.Scan(gitreader.ScanOptions{
				RepoPath:    repoPath,
				Since:       sinceTime,
				Author:      author,
				NoiseFilter: !noFilter,
			})
			if err != nil {
				return fmt.Errorf("scan failed: %w", err)
			}

			mode := gitreader.FormatText
			if jsonOutput {
				mode = gitreader.FormatJSON
			}

			if importFlag {
				return runImport(getStore, result, repoName, dryRun)
			}

			return gitreader.Format(os.Stdout, result, mode, view)
		}
	}

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Extract structured context from local git history",
		Long: `Scan reads your local git history and extracts structured signals —
task groups, ticket IDs, effort estimates — without any API calls or auth.

Subcommands produce different views of the same data:
  kleio scan standup    Today's work summary
  kleio scan pr         PR-style change summary for the current branch
  kleio scan week       Weekly breakdown grouped by day`,
	}

	standupCmd := &cobra.Command{
		Use:   "standup [path]",
		Short: "Generate a daily standup summary from recent commits",
		Args:  cobra.MaximumNArgs(1),
		RunE:  makeRunE(gitreader.ViewStandup),
	}

	prCmd := &cobra.Command{
		Use:   "pr [path]",
		Short: "Generate a PR summary from the current branch",
		Args:  cobra.MaximumNArgs(1),
		RunE:  makeRunE(gitreader.ViewPR),
	}

	weekCmd := &cobra.Command{
		Use:   "week [path]",
		Short: "Generate a weekly breakdown grouped by day",
		Args:  cobra.MaximumNArgs(1),
		RunE:  makeRunE(gitreader.ViewWeek),
	}

	for _, sub := range []*cobra.Command{standupCmd, prCmd, weekCmd} {
		sub.Flags().StringVar(&since, "since", "", "Only include commits after this time (e.g. '3 days ago', '2026-04-20')")
		sub.Flags().StringVar(&author, "author", "", "Filter commits by author email")
		sub.Flags().BoolVar(&jsonOutput, "json", false, "Output as structured JSON")
		sub.Flags().BoolVar(&noFilter, "no-filter-noise", false, "Include merge commits and lockfile changes")
		sub.Flags().BoolVar(&importFlag, "import", false, "Import extracted signals into Kleio")
		sub.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be imported without sending")
		sub.Flags().StringVar(&repoName, "repo", "", "Repository name for imported captures")
		cmd.AddCommand(sub)
	}

	return cmd
}

func defaultSince(view gitreader.ScanView) time.Time {
	now := time.Now()
	switch view {
	case gitreader.ViewStandup:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case gitreader.ViewWeek:
		return now.AddDate(0, 0, -7)
	default:
		return now.AddDate(0, 0, -30)
	}
}

func parseSince(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return parseRelativeTime(s)
}

func parseRelativeTime(s string) (time.Time, error) {
	now := time.Now()
	var n int
	var unit string
	if _, err := fmt.Sscanf(s, "%d %s ago", &n, &unit); err == nil {
		switch {
		case startsWith(unit, "day"):
			return now.AddDate(0, 0, -n), nil
		case startsWith(unit, "week"):
			return now.AddDate(0, 0, -7*n), nil
		case startsWith(unit, "month"):
			return now.AddDate(0, -n, 0), nil
		case startsWith(unit, "hour"):
			return now.Add(-time.Duration(n) * time.Hour), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q (try '3 days ago' or '2026-01-15')", s)
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func runImport(getStore func() kleio.Store, result *gitreader.ScanResult, repoName string, dryRun bool) error {
	pf := privacy.NewFilter(privacy.DefaultRules())
	if len(result.Tasks) == 0 {
		fmt.Println("No tasks to import.")
		return nil
	}

	if dryRun {
		fmt.Printf("Dry run: would import %d tasks (%d commits) as captures\n", len(result.Tasks), len(result.Commits))
		for _, t := range result.Tasks {
			ticketStr := ""
			if len(t.Tickets) > 0 {
				ticketStr = " [" + joinStrings(t.Tickets, ", ") + "]"
			}
			fmt.Printf("  • %s%s (%d commits)\n", t.Summary, ticketStr, len(t.Commits))
		}
		return nil
	}

	store := getStore()
	ctx := context.Background()
	imported := 0
	for _, t := range result.Tasks {
		content := pf.Redact(t.Summary)
		if len(t.Tickets) > 0 {
			content += " [" + joinStrings(t.Tickets, ", ") + "]"
		}

		evt := buildScanEvent(t, content, repoName)
		if err := store.CreateEvent(ctx, evt); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to import task %q: %v\n", t.Summary, err)
			continue
		}
		imported++
	}

	fmt.Printf("Imported %d/%d tasks as captures.\n", imported, len(result.Tasks))
	return nil
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func buildScanEvent(task gitreader.Task, content, repoName string) *kleio.Event {
	var shas []string
	for _, c := range task.Commits {
		if c.Hash != "" {
			shas = append(shas, c.Hash)
		}
	}

	sd := map[string]interface{}{
		"ingest_source": "local_git",
	}
	if len(shas) == 1 {
		sd["commit_sha"] = shas[0]
	} else if len(shas) > 1 {
		sd["commit_shas"] = shas
	}
	if task.Branch != "" {
		sd["branch"] = task.Branch
	}
	sdJSON, _ := json.Marshal(sd)

	provenance := "Imported from local git history"
	if task.Branch != "" {
		provenance += " (branch: " + task.Branch + ")"
	}
	if len(shas) > 0 {
		provenance += " — " + strings.Join(abbreviateSHAs(shas), ", ")
	}

	evt := &kleio.Event{
		Content:         content,
		SignalType:      kleio.SignalTypeWorkItem,
		SourceType:      kleio.SourceTypeCLI,
		StructuredData:  string(sdJSON),
		FreeformContext: provenance,
		RepoName:        repoName,
	}
	return evt
}

func abbreviateSHAs(shas []string) []string {
	out := make([]string, len(shas))
	for i, s := range shas {
		if len(s) > 8 {
			out[i] = s[:8]
		} else {
			out[i] = s
		}
	}
	return out
}
