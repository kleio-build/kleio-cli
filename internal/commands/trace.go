package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/ai"
	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/spf13/cobra"
)

// NewTraceCmd creates the `kleio trace` command.
func NewTraceCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		since         string
		asJSON        bool
		noInteractive bool
	)

	cmd := &cobra.Command{
		Use:   "trace <anchor>",
		Short: "Trace how a file, feature, or topic evolved over time",
		Long: `Reconstruct the chronological evolution of a topic, file, or feature.

Examples:
  kleio trace src/auth/service.go
  kleio trace "checkout flow"
  kleio trace --since 7d "JWT refresh"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			anchor := strings.Join(args, " ")
			isFiletrace := false
			if len(args) >= 2 && args[0] == "file" {
				anchor = strings.Join(args[1:], " ")
				isFiletrace = true
			}
			store := getStore()

			provider, _ := ai.ResolveProvider(ai.LoadConfig())
			eng := engine.New(store, provider)

			sinceTime, _ := parseSince(since)

			var entries []engine.TimelineEntry
			var err error
			if isFiletrace {
				entries, err = eng.FileTimeline(context.Background(), anchor, sinceTime)
			} else {
				entries, err = eng.Timeline(context.Background(), anchor, sinceTime)
			}
			if err != nil {
				return fmt.Errorf("trace failed: %w", err)
			}

			if len(entries) == 0 {
				if asJSON {
					json.NewEncoder(os.Stdout).Encode([]engine.TimelineEntry{})
					os.Exit(1)
				}
				if !noInteractive && isInteractive() {
					entries = runTraceRefinement(os.Stdin, os.Stderr, store, anchor, sinceTime)
				}
				if len(entries) == 0 {
					fmt.Fprintf(os.Stderr, "No results found for %q. Try broadening your search.\n", anchor)
					os.Exit(1)
				}
			}

			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(entries)
			}

			return renderTraceReport(os.Stdout, anchor, entries, eng)
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "Time window filter (e.g. 24h, 7d, 3m)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON for CI/CD pipelines")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "Suppress prompts")

	return cmd
}

func renderTraceReport(w *os.File, anchor string, entries []engine.TimelineEntry, eng *engine.Engine) error {
	fmt.Fprintf(w, "=== Trace Report: %s ===\n\n", anchor)
	fmt.Fprintf(w, "Found %d events across the timeline.\n\n", len(entries))

	segments := segmentTimeline(entries)
	for _, seg := range segments {
		fmt.Fprintf(w, "--- %s ---\n", seg.Label)
		for _, e := range seg.Entries {
			ts := e.Timestamp.Format("2006-01-02 15:04")
			icon := kindIcon(e.Kind)
			fmt.Fprintf(w, "  %s %s %s\n", icon, ts, e.Summary)
		}
		fmt.Fprintln(w)
	}

	var summaryEvents []kleio.Event
	for _, e := range entries {
		summaryEvents = append(summaryEvents, kleio.Event{
			SignalType: e.Kind,
			Content:    e.Summary,
			CreatedAt:  e.Timestamp.Format(time.RFC3339),
		})
	}
	summaryText, _ := eng.Summarize(context.Background(), summaryEvents)
	if summaryText != "" && summaryText != "No events to summarize." {
		fmt.Fprintf(w, "Summary: %s\n", summaryText)
	}

	return nil
}

type segment struct {
	Label   string
	Entries []engine.TimelineEntry
}

func segmentTimeline(entries []engine.TimelineEntry) []segment {
	if len(entries) == 0 {
		return nil
	}

	var segments []segment
	var current segment
	var lastDate string

	for _, e := range entries {
		date := e.Timestamp.Format("2006-01-02")
		if date != lastDate {
			if len(current.Entries) > 0 {
				segments = append(segments, current)
			}
			current = segment{Label: date}
			lastDate = date
		}
		current.Entries = append(current.Entries, e)
	}
	if len(current.Entries) > 0 {
		segments = append(segments, current)
	}

	return segments
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
