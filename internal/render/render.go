package render

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kleio-build/kleio-cli/internal/engine"
)

// Options controls adaptive rendering behaviour.
type Options struct {
	Verbose   bool
	RenderCap int
	DeferCap  int
}

// DefaultOptions returns safe rendering defaults.
func DefaultOptions() Options {
	return Options{
		RenderCap: 10,
		DeferCap:  5,
	}
}

// Render dispatches to the appropriate renderer based on format.
// When output is empty, text/md/json go to w (usually stdout).
// pdf/html without output write to a default file in CWD.
func Render(w io.Writer, format, output string, r engine.Report, verbose bool) error {
	opts := DefaultOptions()
	opts.Verbose = verbose
	return RenderWithOptions(w, format, output, r, opts)
}

// RenderWithOptions dispatches with full adaptive control.
func RenderWithOptions(w io.Writer, format, output string, r engine.Report, opts Options) error {
	if opts.RenderCap <= 0 {
		opts.RenderCap = 10
	}
	if opts.DeferCap <= 0 {
		opts.DeferCap = 5
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "text"
	}

	switch format {
	case "text":
		return renderTo(w, output, ".txt", r, func(out io.Writer) error {
			return RenderText(out, r, opts)
		})
	case "md", "markdown":
		return renderTo(w, output, ".md", r, func(out io.Writer) error {
			return RenderMarkdown(out, r, opts)
		})
	case "html":
		return renderTo(w, output, ".html", r, func(out io.Writer) error {
			return RenderHTML(out, r, opts)
		})
	case "pdf":
		return renderTo(w, output, ".pdf", r, func(out io.Writer) error {
			return RenderPDF(out, r, opts)
		})
	case "json":
		return renderTo(w, output, ".json", r, func(out io.Writer) error {
			return RenderJSON(out, r)
		})
	default:
		return fmt.Errorf("unknown format %q (valid: text, md, html, pdf, json)", format)
	}
}

func renderTo(fallback io.Writer, output, ext string, r engine.Report, fn func(io.Writer) error) error {
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return err
		}
		defer f.Close()
		return fn(f)
	}

	needsFile := ext == ".pdf" || ext == ".html"
	if needsFile {
		slug := slugify(r.Anchor)
		name := fmt.Sprintf("kleio-%s-%s-%d%s", r.Command, slug, time.Now().Unix(), ext)
		path := filepath.Join(".", name)
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := fn(f); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Wrote %s\n", path)
		return nil
	}

	return fn(fallback)
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else if c == ' ' || c == '/' || c == '\\' || c == '_' || c == '.' {
			b.WriteRune('-')
		}
	}
	return b.String()
}
