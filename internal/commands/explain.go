package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/ai"
	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/kleio-build/kleio-cli/internal/gitreader"
	"github.com/spf13/cobra"
)

// ExplainReport is the structured output of `kleio explain`.
type ExplainReport struct {
	Source     string            `json:"source"`
	Target     string            `json:"target"`
	Commits    int               `json:"commits"`
	Subsystems map[string]int    `json:"subsystems"`
	Decisions  []string          `json:"decisions,omitempty"`
	Summary    string            `json:"summary"`
	Details    []ExplainDetail   `json:"details"`
}

// ExplainDetail is a single change group.
type ExplainDetail struct {
	Subsystem string   `json:"subsystem"`
	Files     []string `json:"files"`
	Summary   string   `json:"summary"`
}

// NewExplainCmd creates the `kleio explain` command.
func NewExplainCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		asJSON        bool
		since         string
		noInteractive bool
	)

	cmd := &cobra.Command{
		Use:   "explain <source> <target>",
		Short: "Explain what changed between two points and why",
		Long: `Compare two git refs, branches, or tags and explain changes.

Examples:
  kleio explain HEAD~5 HEAD
  kleio explain main feature/auth-refactor
  kleio explain v1.2.0 v1.3.0`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			source, target := args[0], args[1]
			store := getStore()

			provider, _ := ai.ResolveProvider(ai.LoadConfig())
			eng := engine.New(store, provider)
			_ = noInteractive

			rangeCommits, err := gitreader.CommitRange(".", source, target)
			if err != nil {
				sinceTime, _ := parseSince(since)
				entries, tlErr := eng.Timeline(context.Background(), "", sinceTime)
				if tlErr != nil {
					return fmt.Errorf("explain failed: %w", tlErr)
				}
				report := buildExplainReport(source, target, entries, eng)
				if asJSON {
					return json.NewEncoder(os.Stdout).Encode(report)
				}
				return renderExplainReport(os.Stdout, report)
			}

			var entries []engine.TimelineEntry
			for _, rc := range rangeCommits {
				entries = append(entries, engine.TimelineEntry{
					Timestamp: rc.Timestamp,
					Kind:      "commit",
					Summary:   firstLine(rc.Message),
					SHA:       rc.Hash,
					FilePaths: rc.Files,
				})
			}

			events, _ := store.ListEvents(context.Background(), kleio.EventFilter{Limit: 200})
			for _, ev := range events {
				t, _ := time.Parse(time.RFC3339, ev.CreatedAt)
				entries = append(entries, engine.TimelineEntry{
					Timestamp: t,
					Kind:      "event",
					Summary:   firstLine(ev.Content),
					EventID:   ev.ID,
				})
			}

			report := buildExplainReport(source, target, entries, eng)
			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(report)
			}
			return renderExplainReport(os.Stdout, report)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().StringVar(&since, "since", "", "Time window filter")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "Suppress prompts")

	return cmd
}

func buildExplainReport(source, target string, entries []engine.TimelineEntry, eng *engine.Engine) *ExplainReport {
	subsystems := map[string]int{}
	filesBySubsys := map[string][]string{}
	var commitCount int
	var decisions []string

	for _, e := range entries {
		if e.Kind == kleio.SignalTypeGitCommit {
			commitCount++
			for _, f := range e.FilePaths {
				subsys := extractSubsystem(f)
				subsystems[subsys]++
				filesBySubsys[subsys] = appendUnique(filesBySubsys[subsys], f)
			}
		}
		if e.Kind == kleio.SignalTypeDecision {
			decisions = append(decisions, e.Summary)
		}
	}

	var details []ExplainDetail
	for subsys, files := range filesBySubsys {
		details = append(details, ExplainDetail{
			Subsystem: subsys,
			Files:     files,
			Summary:   fmt.Sprintf("%d file(s) changed", len(files)),
		})
	}
	sort.Slice(details, func(i, j int) bool {
		return subsystems[details[i].Subsystem] > subsystems[details[j].Subsystem]
	})

	var summaryEvents []kleio.Event
	for _, e := range entries {
		summaryEvents = append(summaryEvents, kleio.Event{
			SignalType: e.Kind,
			Content:    e.Summary,
			CreatedAt:  e.Timestamp.Format(time.RFC3339),
		})
	}

	summary := fmt.Sprintf("%d commit(s) across %d subsystem(s)", commitCount, len(subsystems))
	if summaryText, err := eng.Summarize(context.Background(), summaryEvents); err == nil && summaryText != "No events to summarize." {
		summary = summaryText
	}

	return &ExplainReport{
		Source:     source,
		Target:     target,
		Commits:    commitCount,
		Subsystems: subsystems,
		Decisions:  decisions,
		Summary:    summary,
		Details:    details,
	}
}

func renderExplainReport(w *os.File, report *ExplainReport) error {
	fmt.Fprintf(w, "=== Explain: %s -> %s ===\n\n", report.Source, report.Target)
	fmt.Fprintf(w, "Commits: %d\n", report.Commits)
	fmt.Fprintf(w, "Subsystems: %d\n\n", len(report.Subsystems))

	if len(report.Decisions) > 0 {
		fmt.Fprintln(w, "Key Decisions:")
		for _, d := range report.Decisions {
			fmt.Fprintf(w, "  - %s\n", d)
		}
		fmt.Fprintln(w)
	}

	if len(report.Details) > 0 {
		fmt.Fprintln(w, "Changes by Subsystem:")
		for _, d := range report.Details {
			fmt.Fprintf(w, "  [%s] %s\n", d.Subsystem, d.Summary)
			for _, f := range d.Files {
				fmt.Fprintf(w, "    - %s\n", f)
			}
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "Summary: %s\n", report.Summary)
	return nil
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
