package render

import (
	"fmt"
	"io"

	"github.com/kleio-build/kleio-cli/internal/engine"
)

// RenderText writes a human-readable plain-text report to w.
func RenderText(w io.Writer, r engine.Report, verbose bool) error {
	fmt.Fprintf(w, "=== %s Report: %s ===\n\n", capitalize(r.Command), r.Anchor)

	if r.Enriched {
		fmt.Fprintf(w, "[enriched by LLM]\n\n")
	}

	fmt.Fprintf(w, "About\n  %s\n\n", r.Subject)

	sections := sectionOrder(r.Command)
	for _, sec := range sections {
		switch sec {
		case "decisions":
			if len(r.Decisions) == 0 {
				continue
			}
			fmt.Fprintln(w, "Decisions")
			for _, d := range r.Decisions {
				fmt.Fprintf(w, "  • %s\n", d.Content)
				if d.Rationale != "" {
					fmt.Fprintf(w, "    rationale: %s\n", d.Rationale)
				}
			}
			fmt.Fprintln(w)

		case "open_threads":
			if len(r.OpenThreads) == 0 {
				continue
			}
			label := "Open Threads"
			if r.Command == "explain" {
				label = "Review Risks"
			}
			fmt.Fprintln(w, label)
			for _, t := range r.OpenThreads {
				suffix := ""
				if t.Deferred {
					suffix = " [deferred]"
				}
				fmt.Fprintf(w, "  • %s (×%d)%s\n", t.Content, t.Occurrences, suffix)
			}
			fmt.Fprintln(w)

		case "code_changes":
			if len(r.CodeChanges) == 0 {
				continue
			}
			fmt.Fprintln(w, "Code Changes")
			for _, c := range r.CodeChanges {
				sha := c.SHA
				if len(sha) > 7 {
					sha = sha[:7]
				}
				fmt.Fprintf(w, "  %s %s %s\n", sha, c.Date, c.Subject)
			}
			fmt.Fprintln(w)

		case "evidence_quality":
			fmt.Fprintln(w, "Evidence Quality")
			fmt.Fprintf(w, "  fidelity: %s\n", r.EvidenceQuality.HistoryFidelity)
			for src, n := range r.EvidenceQuality.SourceCounts {
				fmt.Fprintf(w, "  %s: %d\n", src, n)
			}
			for _, note := range r.EvidenceQuality.Notes {
				fmt.Fprintf(w, "  note: %s\n", note)
			}
			fmt.Fprintln(w)
		}
	}

	if len(r.NextSteps) > 0 {
		fmt.Fprintln(w, "Next Steps")
		for _, s := range r.NextSteps {
			fmt.Fprintf(w, "  → %s\n", s)
		}
		fmt.Fprintln(w)
	}

	if verbose && len(r.RawTimeline) > 0 {
		fmt.Fprintln(w, "Raw Timeline")
		for _, e := range r.RawTimeline {
			ts := e.Timestamp.Format("2006-01-02 15:04")
			fmt.Fprintf(w, "  [%s] %s %s\n", e.Kind, ts, e.Summary)
		}
		fmt.Fprintln(w)
	}

	return nil
}

func sectionOrder(command string) []string {
	switch command {
	case "explain":
		return []string{"code_changes", "decisions", "open_threads", "evidence_quality"}
	case "incident":
		return []string{"code_changes", "decisions", "evidence_quality"}
	default:
		return []string{"decisions", "open_threads", "code_changes", "evidence_quality"}
	}
}
