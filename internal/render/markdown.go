package render

import (
	"fmt"
	"io"

	"github.com/kleio-build/kleio-cli/internal/engine"
)

// RenderMarkdown writes a markdown-formatted report to w.
func RenderMarkdown(w io.Writer, r engine.Report, opts Options) error {
	fmt.Fprintf(w, "# %s Report: %s\n\n", capitalize(r.Command), r.Anchor)

	if r.Enriched {
		fmt.Fprintf(w, "_Enriched by LLM_\n\n")
	}

	fmt.Fprintf(w, "## About\n\n%s\n\n", r.Subject)

	sections := sectionOrder(r.Command)
	for _, sec := range sections {
		switch sec {
		case "decisions":
			if len(r.Decisions) == 0 {
				continue
			}
			fmt.Fprintf(w, "## Decisions\n\n")
			for _, d := range r.Decisions {
				fmt.Fprintf(w, "- **%s**\n", d.Content)
				if d.Rationale != "" {
					fmt.Fprintf(w, "  - Rationale: %s\n", d.Rationale)
				}
			}
			fmt.Fprintln(w)

		case "open_threads":
			if len(r.OpenThreads) == 0 {
				continue
			}
			heading := "Open Threads"
			if r.Command == "explain" {
				heading = "Related Work"
			}
			fmt.Fprintf(w, "## %s\n\n", heading)
			renderMarkdownThreads(w, r, opts)
			fmt.Fprintln(w)

		case "code_changes":
			if len(r.CodeChanges) == 0 {
				continue
			}
			fmt.Fprintf(w, "## Code Changes\n\n")
			fmt.Fprintln(w, "| SHA | Date | Subject |")
			fmt.Fprintln(w, "|-----|------|---------|")
			for _, c := range r.CodeChanges {
				sha := c.SHA
				if len(sha) > 7 {
					sha = sha[:7]
				}
				fmt.Fprintf(w, "| `%s` | %s | %s |\n", sha, c.Date, c.Subject)
			}
			fmt.Fprintln(w)

		case "evidence_quality":
			fmt.Fprintf(w, "## Evidence Quality\n\n")
			fmt.Fprintf(w, "- **Fidelity**: %s\n", r.EvidenceQuality.HistoryFidelity)
			for src, n := range r.EvidenceQuality.SourceCounts {
				fmt.Fprintf(w, "- %s: %d\n", src, n)
			}
			for _, note := range r.EvidenceQuality.Notes {
				fmt.Fprintf(w, "- _%s_\n", note)
			}
			fmt.Fprintln(w)
		}
	}

	if len(r.NextSteps) > 0 {
		fmt.Fprintf(w, "## Next Steps\n\n")
		for _, s := range r.NextSteps {
			fmt.Fprintf(w, "1. `%s`\n", s)
		}
		fmt.Fprintln(w)
	}

	if opts.Verbose && len(r.RawTimeline) > 0 {
		fmt.Fprintf(w, "## Raw Timeline\n\n")
		fmt.Fprintln(w, "```")
		for _, e := range r.RawTimeline {
			ts := e.Timestamp.Format("2006-01-02 15:04")
			fmt.Fprintf(w, "[%s] %s %s\n", e.Kind, ts, e.Summary)
		}
		fmt.Fprintln(w, "```")
		fmt.Fprintln(w)
	}

	return nil
}

func renderMarkdownThreads(w io.Writer, r engine.Report, opts Options) {
	useGroups := len(r.ThreadGroups) > 1

	if useGroups {
		for _, g := range r.ThreadGroups {
			if g.PlanName != "" {
				fmt.Fprintf(w, "### %s\n\n", g.PlanName)
			}
			renderMarkdownThreadList(w, g.Threads, opts)
		}
	} else {
		renderMarkdownThreadList(w, r.OpenThreads, opts)
	}
}

func renderMarkdownThreadList(w io.Writer, threads []engine.ReportThread, opts Options) {
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
		fmt.Fprintf(w, "- %s (x%d)\n", t.Content, t.Occurrences)
		shown++
	}
	if remaining := len(active) - shown; remaining > 0 {
		fmt.Fprintf(w, "\n_... and %d more (use --verbose to see all)_\n\n", remaining)
	}

	if len(deferred) > 0 {
		fmt.Fprintf(w, "\n**Deferred:**\n\n")
		deferCap := opts.DeferCap
		if opts.Verbose {
			deferCap = len(deferred)
		}
		dShown := 0
		for i, t := range deferred {
			if i >= deferCap {
				break
			}
			fmt.Fprintf(w, "- ~~%s~~ (x%d) _(deferred)_\n", t.Content, t.Occurrences)
			dShown++
		}
		if remaining := len(deferred) - dShown; remaining > 0 {
			fmt.Fprintf(w, "\n_... and %d more deferred_\n", remaining)
		}
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}
