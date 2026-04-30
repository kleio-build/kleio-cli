package engine

import (
	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/ai"
)

// Engine is the shared intelligence layer that powers trace, explain, and
// incident. All operations have a heuristic fallback and optionally
// leverage a BYOK LLM provider.
type Engine struct {
	store kleio.Store
	ai    ai.Provider
}

// New creates an Engine. When provider is nil, Noop is used.
func New(store kleio.Store, provider ai.Provider) *Engine {
	if provider == nil {
		provider = ai.Noop{}
	}
	return &Engine{store: store, ai: provider}
}
