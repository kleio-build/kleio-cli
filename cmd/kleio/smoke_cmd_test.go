package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dslipak/pdf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSmokeReport_RendersAllFormats verifies that runSmokeRender produces
// every artifact and reports zero defects against the synthetic fixture.
// This locks in AC0.4.* criteria inside CI.
func TestSmokeReport_RendersAllFormats(t *testing.T) {
	tmp := t.TempDir()
	r := smokeFixtureReport()

	defects := runSmokeRender(r, tmp)
	if len(defects) > 0 {
		t.Fatalf("expected zero defects, got %d:\n  - %s", len(defects), strings.Join(defects, "\n  - "))
	}

	for _, ext := range []string{"txt", "md", "html", "pdf", "json"} {
		path := filepath.Join(tmp, "smoke."+ext)
		info, err := os.Stat(path)
		require.NoError(t, err, "missing %s", path)
		assert.Greater(t, info.Size(), int64(50), "%s should be non-trivial", path)
	}
}

// TestSmokeReport_PDFHasNoMojibake confirms that the PDF byte stream
// produced by the smoke fixture (which deliberately injects UTF-8 bullets,
// arrows, em-dashes, smart quotes, and ellipses) contains zero of those
// sequences after asciiSafe normalization.
func TestSmokeReport_PDFHasNoMojibake(t *testing.T) {
	tmp := t.TempDir()
	r := smokeFixtureReport()
	defects := runSmokeRender(r, tmp)
	require.Empty(t, defects, "smoke render reported defects")

	raw, err := os.ReadFile(filepath.Join(tmp, "smoke.pdf"))
	require.NoError(t, err)

	mojibake := map[string][]byte{
		"bullet":      {0xE2, 0x80, 0xA2},
		"right-arrow": {0xE2, 0x86, 0x92},
		"em-dash":     {0xE2, 0x80, 0x94},
		"smart-quote": {0xE2, 0x80, 0x9C},
		"ellipsis":    {0xE2, 0x80, 0xA6},
	}
	for name, seq := range mojibake {
		assert.False(t, bytes.Contains(raw, seq), "PDF byte stream contains UTF-8 %s sequence", name)
	}
}

// TestSmokeReport_PDFReadBackContainsAnchor validates the deepest part of
// the audit pipeline: a third-party PDF parser can extract plain text and
// the rendered anchor survives encoding-normalization.
func TestSmokeReport_PDFReadBackContainsAnchor(t *testing.T) {
	tmp := t.TempDir()
	r := smokeFixtureReport()
	require.Empty(t, runSmokeRender(r, tmp))

	raw, err := os.ReadFile(filepath.Join(tmp, "smoke.pdf"))
	require.NoError(t, err)
	doc, err := pdf.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	var b strings.Builder
	for i := 1; i <= doc.NumPage(); i++ {
		p := doc.Page(i)
		if p.V.IsNull() {
			continue
		}
		txt, _ := p.GetPlainText(nil)
		b.WriteString(txt)
	}
	text := b.String()
	assert.Contains(t, text, r.Anchor, "PDF plain text should include anchor")
	assert.Contains(t, text, "Decisions", "PDF should expose section headings")
	assert.Contains(t, text, "Code Changes")
	assert.Contains(t, text, "Next Steps")
}
