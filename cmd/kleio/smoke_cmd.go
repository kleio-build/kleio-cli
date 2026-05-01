package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dslipak/pdf"
	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/kleio-build/kleio-cli/internal/render"
	"github.com/spf13/cobra"
)

// newDevCmd returns the hidden `kleio dev` command tree. Sub-commands here
// are intended for contributor workflows (E2E smoke tests, golden file
// regeneration, etc.) and are not surfaced in the standard help output.
func newDevCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "dev",
		Hidden: true,
		Short:  "Developer-only diagnostic commands (not part of public CLI)",
	}
	cmd.AddCommand(newSmokeReportCmd())
	cmd.AddCommand(newDevIngestCmd())
	cmd.AddCommand(newDevCorrelateCmd())
	cmd.AddCommand(newDevSynthesizeCmd())
	return cmd
}

// newSmokeReportCmd renders a synthetic Report through every supported
// format and validates the output. The PDF is opened with dslipak/pdf and
// its byte stream scanned for the `\xE2\x80\xA2` UTF-8 bullet sequence to
// guard against regression of the PDF mojibake bug.
//
// Usage:
//
//	kleio dev smoke-report                  # render to ./kleio-smoke/
//	kleio dev smoke-report --out ./out      # custom output dir
//	kleio dev smoke-report --strict         # exit non-zero on any defect
func newSmokeReportCmd() *cobra.Command {
	var (
		outDir string
		strict bool
	)
	cmd := &cobra.Command{
		Use:   "smoke-report",
		Short: "End-to-end render-validate sweep across text/md/html/pdf/json",
		Long: `Renders a synthetic Report across all supported output formats, validates
each, and reports defects. The PDF is opened with a third-party reader and
scanned for known mojibake byte sequences (e.g. UTF-8 bullet \xE2\x80\xA2)
that break under fpdf's default WinAnsi encoding.

Exits 0 on success, 1 on validation failure when --strict is set.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if outDir == "" {
				outDir = "kleio-smoke"
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return fmt.Errorf("create output dir: %w", err)
			}

			r := smokeFixtureReport()
			defects := runSmokeRender(r, outDir)

			fmt.Println("kleio dev smoke-report:")
			fmt.Printf("  output dir: %s\n", outDir)
			if len(defects) == 0 {
				fmt.Println("  status:     OK (all formats rendered and validated)")
				return nil
			}
			fmt.Printf("  status:     %d defect(s)\n", len(defects))
			for _, d := range defects {
				fmt.Printf("  - %s\n", d)
			}
			if strict {
				return fmt.Errorf("smoke-report found %d defect(s)", len(defects))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "", "directory to write rendered artifacts to (default: ./kleio-smoke)")
	cmd.Flags().BoolVar(&strict, "strict", false, "exit non-zero when any defect is detected")
	return cmd
}

// smokeFixtureReport returns a Report packed with content designed to
// exercise every renderer's edge cases:
//   - long subjects (must wrap in PDF / HTML)
//   - UTF-8 punctuation that triggers PDF mojibake when not normalized
//   - multiple decisions/threads/code changes
//   - a deferred thread (suffix rendering)
//   - evidence quality with notes
func smokeFixtureReport() engine.Report {
	long := strings.Repeat("very-long-commit-subject-that-must-wrap ", 8)
	return engine.Report{
		Anchor:      "auth",
		Command:     "trace",
		GeneratedAt: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		Subject: "Trace report fixture covering long subjects \u2192 wrap, " +
			"smart \u201Cquotes\u201D, em\u2014dashes, en\u2013dashes, ellipsis\u2026 and \u2022 bullets.",
		Decisions: []engine.ReportDecision{
			{
				Content:   "Adopt JWT instead of session cookies for the public API surface",
				Rationale: "Stateless verification simplifies horizontal scaling and removes Redis dependency",
				At:        "2026-04-30T10:00:00Z",
			},
			{
				Content: "Defer rate-limit middleware until after launch \u2014 measured baseline first",
				At:      "2026-04-29T16:00:00Z",
			},
		},
		OpenThreads: []engine.ReportThread{
			{Content: long, Occurrences: 4},
			{Content: "Audit log retention window not yet defined", Occurrences: 2},
			{Content: "Defer caching layer; revisit once query latency is profiled", Occurrences: 1, Deferred: true},
		},
		CodeChanges: []engine.ReportChange{
			{SHA: "abc1234567", Date: "2026-04-30", Subject: long, Files: []string{"internal/auth.go"}},
			{SHA: "deadbeef00", Date: "2026-04-29", Subject: "feat: introduce JWT verifier", Files: []string{"internal/auth.go"}},
		},
		EvidenceQuality: engine.EvidenceQuality{
			SourceCounts:    map[string]int{"mcp": 4, "local_git": 12, "cursor_transcript": 7},
			HistoryFidelity: "high",
			Notes:           []string{"3 work item(s) appear duplicated across re-imported transcripts."},
		},
		NextSteps: []string{
			"kleio explain abc1234 HEAD",
			`kleio backlog list --search "auth"`,
			"kleio incident \"jwt verifier\"",
		},
		RawTimeline: []engine.TimelineEntry{
			{Timestamp: time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC), Kind: "decision", Summary: "Adopt JWT"},
		},
		Enriched: false,
	}
}

// runSmokeRender renders r in every supported format and returns a slice of
// defect descriptions. An empty return value indicates all formats validated.
func runSmokeRender(r engine.Report, outDir string) []string {
	var defects []string

	// text / markdown / json: render to fixed paths so we can read them
	// back uniformly (Render() writes .html and .pdf to disk by default).
	formats := []struct{ name, ext string }{
		{"text", "txt"},
		{"md", "md"},
		{"html", "html"},
		{"json", "json"},
	}
	for _, f := range formats {
		path := filepath.Join(outDir, "smoke."+f.ext)
		if err := render.Render(io.Discard, f.name, path, r, false); err != nil {
			defects = append(defects, fmt.Sprintf("render %s: %v", f.name, err))
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			defects = append(defects, fmt.Sprintf("read %s: %v", path, err))
			continue
		}
		if !strings.Contains(string(body), r.Anchor) {
			defects = append(defects, fmt.Sprintf("%s output missing anchor %q", f.name, r.Anchor))
		}
	}

	// PDF: write to disk, then re-open with dslipak/pdf and assert it
	// parses + contains expected text + has zero UTF-8 mojibake bytes.
	pdfPath := filepath.Join(outDir, "smoke.pdf")
	pf, err := os.Create(pdfPath)
	if err != nil {
		defects = append(defects, fmt.Sprintf("create %s: %v", pdfPath, err))
		return defects
	}
	if err := render.RenderPDF(pf, r, false); err != nil {
		pf.Close()
		defects = append(defects, fmt.Sprintf("render pdf: %v", err))
		return defects
	}
	pf.Close()

	defects = append(defects, validatePDFArtifact(pdfPath, r)...)
	return defects
}

// validatePDFArtifact reads the PDF back from disk and applies three
// classes of check:
//  1. Byte stream contains no UTF-8 mojibake sequences.
//  2. dslipak/pdf can parse and produce non-empty plain text.
//  3. The plain text contains the report anchor.
//
// Each failed check produces one defect string; an empty return is success.
func validatePDFArtifact(path string, r engine.Report) []string {
	var defects []string

	raw, err := os.ReadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("read pdf back: %v", err)}
	}

	mojibake := map[string][]byte{
		"UTF-8 bullet (\\xE2\\x80\\xA2)":     {0xE2, 0x80, 0xA2},
		"UTF-8 right-arrow (\\xE2\\x86\\x92)": {0xE2, 0x86, 0x92},
		"UTF-8 em-dash (\\xE2\\x80\\x94)":     {0xE2, 0x80, 0x94},
		"UTF-8 smart-quote (\\xE2\\x80\\x9C)": {0xE2, 0x80, 0x9C},
		"UTF-8 ellipsis (\\xE2\\x80\\xA6)":    {0xE2, 0x80, 0xA6},
	}
	for name, seq := range mojibake {
		if bytes.Contains(raw, seq) {
			defects = append(defects, "PDF byte stream contains "+name)
		}
	}

	// Parse from in-memory bytes via NewReader + bytes.Reader so we don't
	// hold a Windows file handle that blocks t.TempDir() cleanup.
	doc, err := pdf.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		defects = append(defects, fmt.Sprintf("dslipak/pdf open: %v", err))
		return defects
	}
	var text strings.Builder
	totalPages := doc.NumPage()
	for i := 1; i <= totalPages; i++ {
		page := doc.Page(i)
		if page.V.IsNull() {
			continue
		}
		txt, terr := page.GetPlainText(nil)
		if terr != nil {
			defects = append(defects, fmt.Sprintf("dslipak/pdf page %d text: %v", i, terr))
			continue
		}
		text.WriteString(txt)
	}
	if text.Len() == 0 {
		defects = append(defects, "PDF parsed but extracted zero plain text")
	}
	if !strings.Contains(text.String(), r.Anchor) {
		defects = append(defects, fmt.Sprintf("PDF plain text missing anchor %q", r.Anchor))
	}
	return defects
}
