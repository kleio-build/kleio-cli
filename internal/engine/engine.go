package engine

import (
	"context"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/ai"
)

// AnchorExpander widens a single search anchor into the set of terms
// trace/explain/incident should look up. The empty interface lives here
// (rather than importing internal/aliases) so engine remains alias-free
// for tests; the real implementation is wired in by the cmd layer.
type AnchorExpander interface {
	Expand(ctx context.Context, anchor string) []string
}

// Engine is the shared intelligence layer that powers trace, explain, and
// incident. All operations have a heuristic fallback and optionally
// leverage a BYOK LLM provider.
type Engine struct {
	store    kleio.Store
	ai       ai.Provider
	expander AnchorExpander
}

// New creates an Engine. When provider is nil, Noop is used.
func New(store kleio.Store, provider ai.Provider) *Engine {
	if provider == nil {
		provider = ai.Noop{}
	}
	return &Engine{store: store, ai: provider}
}

// WithExpander attaches an AnchorExpander used by Timeline/FileTimeline
// to widen recall via static aliases (and optional LLM expansion).
// Returns the receiver so callers can chain.
func (e *Engine) WithExpander(exp AnchorExpander) *Engine {
	e.expander = exp
	return e
}
