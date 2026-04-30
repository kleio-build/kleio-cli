package ai

import "context"

// Provider is the abstraction for LLM backends. Every engine function
// checks Available() first and falls back to heuristics when false.
type Provider interface {
	Available() bool
	Complete(ctx context.Context, prompt string) (string, error)
	Embed(ctx context.Context, text string) ([]float64, error)
}
