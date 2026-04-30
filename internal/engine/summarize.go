package engine

import (
	"context"
	"fmt"
	"strings"

	kleio "github.com/kleio-build/kleio-core"
)

// Summarize produces a natural-language summary of the given events.
// Uses LLM when available, otherwise falls back to heuristic extraction.
func (e *Engine) Summarize(ctx context.Context, events []kleio.Event) (string, error) {
	if len(events) == 0 {
		return "No events to summarize.", nil
	}

	if e.ai.Available() {
		return e.llmSummarize(ctx, events)
	}
	return heuristicSummary(events), nil
}

func (e *Engine) llmSummarize(ctx context.Context, events []kleio.Event) (string, error) {
	var sb strings.Builder
	sb.WriteString("Summarize the following engineering events into a concise narrative:\n\n")
	for i, ev := range events {
		if i >= 30 {
			fmt.Fprintf(&sb, "... and %d more events\n", len(events)-30)
			break
		}
		fmt.Fprintf(&sb, "- [%s] %s: %s\n", ev.CreatedAt, ev.SignalType, firstLine(ev.Content))
	}

	result, err := e.ai.Complete(ctx, sb.String())
	if err != nil || result == "" {
		return heuristicSummary(events), nil
	}
	return result, nil
}

func heuristicSummary(events []kleio.Event) string {
	typeCounts := map[string]int{}
	for _, ev := range events {
		typeCounts[ev.SignalType]++
	}

	var parts []string
	if n := typeCounts[kleio.SignalTypeDecision]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d decision(s)", n))
	}
	if n := typeCounts[kleio.SignalTypeCheckpoint]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d checkpoint(s)", n))
	}
	if n := typeCounts[kleio.SignalTypeWorkItem]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d work item(s)", n))
	}
	if n := typeCounts[kleio.SignalTypeGitCommit]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d git commit(s)", n))
	}

	summary := fmt.Sprintf("%d event(s)", len(events))
	if len(parts) > 0 {
		summary += ": " + strings.Join(parts, ", ")
	}

	if len(events) > 0 {
		summary += fmt.Sprintf(". Most recent: %q", firstLine(events[0].Content))
	}

	return summary
}
