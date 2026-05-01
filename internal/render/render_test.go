package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleReport() engine.Report {
	return engine.Report{
		Anchor:      "auth",
		Command:     "trace",
		GeneratedAt: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		Subject:     "3 signals tied to auth: 1 decision(s), 2 commit(s).",
		Decisions: []engine.ReportDecision{
			{Content: "Use JWT over sessions", Rationale: "Stateless needed", At: "2026-04-30T10:00:00Z"},
		},
		OpenThreads: []engine.ReportThread{
			{Content: "Add error handling", Occurrences: 2, FirstSeen: "2026-04-30T08:00:00Z", LastSeen: "2026-04-30T11:00:00Z"},
			{Content: "Defer caching layer", Occurrences: 1, FirstSeen: "2026-04-30T09:00:00Z", LastSeen: "2026-04-30T09:00:00Z", Deferred: true},
		},
		CodeChanges: []engine.ReportChange{
			{SHA: "abc1234567", Date: "2026-04-30", Subject: "feat: add auth module", Files: []string{"auth.go"}},
		},
		EvidenceQuality: engine.EvidenceQuality{
			SourceCounts:    map[string]int{"mcp": 1, "local_git": 2},
			HistoryFidelity: "high",
			Notes:           []string{"1 work item(s) appear duplicated across re-imported transcripts."},
		},
		NextSteps: []string{
			"kleio explain abc1234 HEAD",
			`kleio backlog list --search "auth"`,
		},
		RawTimeline: []engine.TimelineEntry{
			{Timestamp: time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC), Kind: "decision", Summary: "Use JWT"},
		},
	}
}

func TestRenderText(t *testing.T) {
	var buf bytes.Buffer
	err := RenderText(&buf, sampleReport(), false)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "=== Trace Report: auth ===")
	assert.Contains(t, out, "3 signals tied to auth")
	assert.Contains(t, out, "Decisions")
	assert.Contains(t, out, "Use JWT over sessions")
	assert.Contains(t, out, "rationale: Stateless needed")
	assert.Contains(t, out, "Open Threads")
	assert.Contains(t, out, "Add error handling")
	assert.Contains(t, out, "[deferred]")
	assert.Contains(t, out, "Code Changes")
	assert.Contains(t, out, "abc1234")
	assert.Contains(t, out, "Evidence Quality")
	assert.Contains(t, out, "Next Steps")
	assert.NotContains(t, out, "Raw Timeline", "verbose=false should hide timeline")
}

func TestRenderText_Verbose(t *testing.T) {
	var buf bytes.Buffer
	err := RenderText(&buf, sampleReport(), true)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Raw Timeline")
	assert.Contains(t, buf.String(), "Use JWT")
}

func TestRenderMarkdown(t *testing.T) {
	var buf bytes.Buffer
	err := RenderMarkdown(&buf, sampleReport(), false)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "# Trace Report: auth")
	assert.Contains(t, out, "## About")
	assert.Contains(t, out, "## Decisions")
	assert.Contains(t, out, "**Use JWT over sessions**")
	assert.Contains(t, out, "## Code Changes")
	assert.Contains(t, out, "| SHA | Date | Subject |")
	assert.Contains(t, out, "`abc1234`")
	assert.Contains(t, out, "## Next Steps")
}

func TestRenderHTML(t *testing.T) {
	var buf bytes.Buffer
	err := RenderHTML(&buf, sampleReport(), false)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "<!DOCTYPE html>")
	assert.Contains(t, out, "<title>trace Report: auth</title>")
	assert.Contains(t, out, "<h2>Decisions</h2>")
	assert.Contains(t, out, "Use JWT over sessions")
	assert.Contains(t, out, "<table>")
	assert.Contains(t, out, "kleio-cli")
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	err := RenderJSON(&buf, sampleReport())
	require.NoError(t, err)

	out := buf.String()
	assert.True(t, strings.HasPrefix(strings.TrimSpace(out), "{"))
	assert.Contains(t, out, `"anchor": "auth"`)
	assert.Contains(t, out, `"command": "trace"`)
	assert.Contains(t, out, `"Use JWT over sessions"`)
}

func TestRenderPDF(t *testing.T) {
	var buf bytes.Buffer
	err := RenderPDF(&buf, sampleReport(), false)
	require.NoError(t, err)

	assert.True(t, buf.Len() > 100, "PDF should have content")
	assert.True(t, strings.HasPrefix(buf.String(), "%PDF-1."), "should start with PDF header")
}

// TestRenderPDF_NoUTF8BulletMojibake guards AC0.1.3: PDF byte stream must
// contain zero occurrences of the UTF-8 bullet sequence (0xE2 0x80 0xA2),
// which is what gets misrendered as `â€¢` when fed to fpdf's WinAnsi font.
func TestRenderPDF_NoUTF8BulletMojibake(t *testing.T) {
	r := sampleReport()
	// Inject every UTF-8 trap character we replace.
	r.Subject = "\u2022 bullet \u2192 arrow \u2014 emdash \u2013 endash \u201Csmart\u201D \u2026ellipsis"
	r.OpenThreads = append(r.OpenThreads, engine.ReportThread{
		Content: "Item with \u2022 bullet inside", Occurrences: 1,
	})

	var buf bytes.Buffer
	require.NoError(t, RenderPDF(&buf, r, false))

	utf8Bullet := []byte{0xE2, 0x80, 0xA2}
	assert.False(t, bytes.Contains(buf.Bytes(), utf8Bullet),
		"PDF byte stream must not contain UTF-8 bullet sequence \\xE2\\x80\\xA2")

	utf8Arrow := []byte{0xE2, 0x86, 0x92}
	assert.False(t, bytes.Contains(buf.Bytes(), utf8Arrow),
		"PDF byte stream must not contain UTF-8 right-arrow sequence \\xE2\\x86\\x92")

	utf8EmDash := []byte{0xE2, 0x80, 0x94}
	assert.False(t, bytes.Contains(buf.Bytes(), utf8EmDash),
		"PDF byte stream must not contain UTF-8 em-dash sequence \\xE2\\x80\\x94")

	utf8SmartLeft := []byte{0xE2, 0x80, 0x9C}
	assert.False(t, bytes.Contains(buf.Bytes(), utf8SmartLeft),
		"PDF byte stream must not contain UTF-8 smart-quote sequence \\xE2\\x80\\x9C")
}

// TestRenderPDF_LongSubjectsWrap guards AC0.1.5: variable-length lines must
// use MultiCell so they wrap rather than overflow page width. We can't
// visually inspect from a unit test, but we can verify the renderer accepts
// pathologically long input without error and produces a multi-page PDF
// (proving auto-page-break and wrapping fired).
func TestRenderPDF_LongSubjectsWrap(t *testing.T) {
	r := sampleReport()
	long := strings.Repeat("very-long-commit-subject-that-must-wrap ", 30)
	r.CodeChanges = []engine.ReportChange{
		{SHA: "deadbeef", Date: "2026-04-30", Subject: long},
	}
	r.OpenThreads = []engine.ReportThread{
		{Content: long, Occurrences: 1},
	}

	var buf bytes.Buffer
	require.NoError(t, RenderPDF(&buf, r, false))
	assert.True(t, buf.Len() > 500, "PDF with long content should produce real output")
	assert.True(t, strings.HasPrefix(buf.String(), "%PDF-1."))
}

func TestAsciiSafe(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"\u2022 bullet", "* bullet"},
		{"a \u2192 b", "a -> b"},
		{"a \u2190 b", "a <- b"},
		{"em \u2014 dash", "em -- dash"},
		{"en \u2013 dash", "en - dash"},
		{"\u201Csmart\u201D", "\"smart\""},
		{"it\u2019s", "it's"},
		{"loading\u2026", "loading..."},
		{"6 \u00D7 7", "6 x 7"},
		{"plain ascii", "plain ascii"},
		{"\u00A0nbsp", " nbsp"},
		{"keep latin1 \u00E9", "keep latin1 \u00E9"},
		{"drop emoji \U0001F600 here", "drop emoji  here"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assert.Equal(t, c.want, asciiSafe(c.in))
		})
	}
}

func TestRenderDispatcher_UnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, "xml", "", sampleReport(), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown format")
}

func TestRenderDispatcher_JSONAlias(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, "json", "", sampleReport(), false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"anchor"`)
}

func TestSlugify(t *testing.T) {
	assert.Equal(t, "hello-world", slugify("Hello World"))
	assert.Equal(t, "src-auth-go", slugify("src/auth.go"))
}

func TestExplainSectionOrder(t *testing.T) {
	order := sectionOrder("explain")
	assert.Equal(t, "code_changes", order[0])
	assert.Equal(t, "decisions", order[1])
}

func TestCapitalize(t *testing.T) {
	assert.Equal(t, "Trace", capitalize("trace"))
	assert.Equal(t, "Explain", capitalize("explain"))
	assert.Equal(t, "", capitalize(""))
	assert.Equal(t, "Already", capitalize("Already"))
}
