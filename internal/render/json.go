package render

import (
	"encoding/json"
	"io"

	"github.com/kleio-build/kleio-cli/internal/engine"
)

// RenderJSON writes a JSON-encoded report to w.
func RenderJSON(w io.Writer, r engine.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
