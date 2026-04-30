package ai

import "context"

// Noop is returned when no AI provider is configured. All calls succeed
// with empty results, signaling the engine to use heuristic fallbacks.
type Noop struct{}

func (Noop) Available() bool                                     { return false }
func (Noop) Complete(_ context.Context, _ string) (string, error) { return "", nil }
func (Noop) Embed(_ context.Context, _ string) ([]float64, error) { return nil, nil }
