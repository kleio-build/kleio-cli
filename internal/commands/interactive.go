package commands

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/engine"
)

const maxRefinementRounds = 3

func runTraceRefinement(in io.Reader, out io.Writer, store kleio.Store, anchor string, sinceTime time.Time) []engine.TimelineEntry {
	scanner := bufio.NewScanner(in)
	for round := 0; round < maxRefinementRounds; round++ {
		fmt.Fprintf(out, "No results for %q. Suggestions:\n", anchor)
		fmt.Fprintf(out, "  1. Try a different keyword\n")
		fmt.Fprintf(out, "  2. Broaden the time window (current: %s)\n", formatWindow(sinceTime))
		fmt.Fprintf(out, "  3. Quit\n")
		fmt.Fprintf(out, "Enter new search term (or 'q' to quit): ")

		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" || input == "q" || input == "3" {
			break
		}

		newAnchor := input
		eng := engine.New(store, nil)
		entries, err := eng.Timeline(context.Background(), newAnchor, sinceTime)
		if err == nil && len(entries) > 0 {
			fmt.Fprintf(out, "Found %d result(s) for %q.\n\n", len(entries), newAnchor)
			return entries
		}
		anchor = newAnchor
	}
	return nil
}

func runIncidentRefinement(in io.Reader, out io.Writer, eng *engine.Engine, store kleio.Store, signal string, files []string, sinceTime time.Time) *IncidentReport {
	scanner := bufio.NewScanner(in)
	for round := 0; round < maxRefinementRounds; round++ {
		fmt.Fprintf(out, "No suspects found for %q. Suggestions:\n", signal)
		fmt.Fprintf(out, "  1. Try different keywords\n")
		fmt.Fprintf(out, "  2. Expand the time window\n")
		fmt.Fprintf(out, "  3. Quit\n")
		fmt.Fprintf(out, "Enter new signal (or 'q' to quit): ")

		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" || input == "q" || input == "3" {
			break
		}

		report, err := buildIncidentReport(context.Background(), eng, store, input, files, sinceTime)
		if err == nil && len(report.Suspects) > 0 {
			fmt.Fprintf(out, "Found %d suspect(s) for %q.\n\n", len(report.Suspects), input)
			return report
		}
		signal = input
	}
	return nil
}

func formatWindow(since time.Time) string {
	d := time.Since(since)
	if d > 24*time.Hour {
		return fmt.Sprintf("%.0f day(s)", d.Hours()/24)
	}
	return fmt.Sprintf("%.0f hour(s)", d.Hours())
}
