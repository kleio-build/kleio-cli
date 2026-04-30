package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
			store := getStore()

			provider, _ := ai.ResolveProvider(ai.LoadConfig())
			eng := engine.New(store, provider)

			sinceTime, _ := parseSince(since)

			entries, err := eng.Timeline(context.Background(), anchor, sinceTime)
			if err != nil {
				return fmt.Errorf("trace failed: %w", err)
			}

			if len(entries) == 0 {
				if !noInteractive && isInteractive() {
					fmt.Fprintf(os.Stderr, "No results found for %q. Try broadening your search.\n", anchor)
				}
				os.Exit(1)
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

	events, _ := eng.Summarize(context.Background(), nil)
	if events != "" && events != "No events to summarize." {
		fmt.Fprintf(w, "Summary: %s\n", events)
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
	case "commit":
		return "[commit]"
	case "event":
		return "[event]"
	case "link":
		return "[link]"
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
