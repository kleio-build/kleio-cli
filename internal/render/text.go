package render

import (
	"fmt"
	"io"

	"github.com/kleio-build/kleio-cli/internal/engine"
)

// RenderText writes a human-readable plain-text report to w.
func RenderText(w io.Writer, r engine.Report, opts Options) error {
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
				fmt.Fprintf(w, "  * %s\n", d.Content)
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
				label = "Related Work"
			}
			fmt.Fprintln(w, label)
			renderTextThreads(w, r, opts)
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
			fmt.Fprintf(w, "  -> %s\n", s)
		}
		fmt.Fprintln(w)
	}

	if opts.Verbose && len(r.RawTimeline) > 0 {
		fmt.Fprintln(w, "Raw Timeline")
		for _, e := range r.RawTimeline {
			ts := e.Timestamp.Format("2006-01-02 15:04")
			fmt.Fprintf(w, "  [%s] %s %s\n", e.Kind, ts, e.Summary)
		}
		fmt.Fprintln(w)
	}

	return nil
}

func renderTextThreads(w io.Writer, r engine.Report, opts Options) {
	useGroups := len(r.ThreadGroups) > 1

	if useGroups {
		for _, g := range r.ThreadGroups {
			if g.PlanName != "" {
				fmt.Fprintf(w, "\n  -- %s --\n", g.PlanName)
			}
			renderTextThreadList(w, g.Threads, opts)
		}
	} else {
		renderTextThreadList(w, r.OpenThreads, opts)
	}
}

func renderTextThreadList(w io.Writer, threads []engine.ReportThread, opts Options) {
	active, deferred := splitActiveDeferred(threads)

	cap := opts.RenderCap
	if opts.Verbose {
		cap = len(active)
	}
	shown := 0
	for i, t := range active {
		if i >= cap {
			break
		}
		fmt.Fprintf(w, "  * %s (x%d)\n", t.Content, t.Occurrences)
		shown++
	}
	if remaining := len(active) - shown; remaining > 0 {
		fmt.Fprintf(w, "  ... and %d more (use --verbose to see all)\n", remaining)
	}

	if len(deferred) > 0 {
		fmt.Fprintln(w, "  Deferred:")
		deferCap := opts.DeferCap
		if opts.Verbose {
			deferCap = len(deferred)
		}
		dShown := 0
		for i, t := range deferred {
			if i >= deferCap {
				break
			}
			fmt.Fprintf(w, "  * %s (x%d) [deferred]\n", t.Content, t.Occurrences)
			dShown++
		}
		if remaining := len(deferred) - dShown; remaining > 0 {
			fmt.Fprintf(w, "  ... and %d more deferred\n", remaining)
		}
	}
}

func splitActiveDeferred(threads []engine.ReportThread) (active, deferred []engine.ReportThread) {
	for _, t := range threads {
		if t.Deferred {
			deferred = append(deferred, t)
		} else {
			active = append(active, t)
		}
	}
	return
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
