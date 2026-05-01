// Package pipeline assembles the canonical kleio.Pipeline for the CLI.
//
// The Pipeline struct lives in kleio-core (so kleio-app can construct
// the same shape). This package is the CLI's wiring layer: it picks
// concrete ingesters, correlators, and synthesizers based on the
// user's discovery scope, the Store backend, and whether an
// ai.Provider is available.
//
// Auto-promotion rules (per Phase 1.4 user directive):
//
//   - SearchCorrelator ALWAYS wired (FTS5 locally, embeddings on cloud).
//   - EmbedCorrelator REPLACES SearchCorrelator iff ai.Provider available.
//   - LLMSynthesizer added as a final pass when ai.Provider available.
//   - PlanClusterSynthesizer + OrphanSynthesizer always wired (no LLM
//     dependency).
package pipeline

import (
	"time"

	"github.com/kleio-build/kleio-cli/internal/ai"
	"github.com/kleio-build/kleio-cli/internal/correlate/embed"
	"github.com/kleio-build/kleio-cli/internal/correlate/filepath"
	"github.com/kleio-build/kleio-cli/internal/correlate/idreference"
	"github.com/kleio-build/kleio-cli/internal/correlate/search"
	"github.com/kleio-build/kleio-cli/internal/correlate/timewindow"
	"github.com/kleio-build/kleio-cli/internal/ingest/discovery"
	"github.com/kleio-build/kleio-cli/internal/synthesize/llm"
	"github.com/kleio-build/kleio-cli/internal/synthesize/orphan"
	"github.com/kleio-build/kleio-cli/internal/synthesize/plancluster"
	kleio "github.com/kleio-build/kleio-core"
)

// Config controls the pipeline assembly. Most fields have sensible
// defaults; tests and CLI commands override on demand.
type Config struct {
	// Discovery resolves which plans, transcripts, and repos to ingest.
	Discovery discovery.Discovery

	// Store is the persistence backend (localdb.Store or
	// apistore.Store). Required.
	Store kleio.Store

	// Provider is the LLM backend. nil or unavailable -> no LLM
	// promotion happens; SearchCorrelator stays as the semantic
	// correlator.
	Provider ai.Provider

	// TimeWindow is the bucket size for TimeWindowCorrelator. Default
	// 15min when zero.
	TimeWindow time.Duration

	// EnabledIngesters subsets which ingesters run. nil means
	// "everything" (plan + transcript + git). Use this to scope
	// per-source ingest runs (e.g. `kleio ingest --source plan`).
	EnabledIngesters map[string]bool

	// EnabledCorrelators / EnabledSynthesizers behave the same way.
	EnabledCorrelators  map[string]bool
	EnabledSynthesizers map[string]bool
}

// Build assembles a kleio.Pipeline ready to Run. It does NOT run
// anything; callers invoke Pipeline.Run with their IngestScope.
//
// Build is intentionally side-effect-free: no DB writes, no LLM
// pings (ai.AutoDetect is the caller's responsibility).
func Build(cfg Config) *kleio.Pipeline {
	if cfg.TimeWindow <= 0 {
		cfg.TimeWindow = 15 * time.Minute
	}

	ingesters := allIngesters(cfg.Discovery)
	correlators := allCorrelators(cfg)
	synthesizers := allSynthesizers(cfg)

	if cfg.EnabledIngesters != nil {
		ingesters = filterByName(ingesters, cfg.EnabledIngesters, ingesterName)
	}
	if cfg.EnabledCorrelators != nil {
		correlators = filterByName(correlators, cfg.EnabledCorrelators, correlatorName)
	}
	if cfg.EnabledSynthesizers != nil {
		synthesizers = filterByName(synthesizers, cfg.EnabledSynthesizers, synthesizerName)
	}

	return &kleio.Pipeline{
		Ingesters:    ingesters,
		Correlators:  correlators,
		Synthesizers: synthesizers,
		Store:        cfg.Store,
	}
}

func allIngesters(d discovery.Discovery) []kleio.Ingester {
	return []kleio.Ingester{
		d.PlanIngester(),
		d.TranscriptIngester(),
		d.GitIngester(),
	}
}

// allCorrelators returns the canonical correlator stack with the
// embed/search auto-promotion rule applied. Order matters because
// downstream synthesizers see the union of all clusters: cheap
// correlators (time, id, file) run first so the more expensive
// semantic pass can de-prioritise duplicates.
func allCorrelators(cfg Config) []kleio.Correlator {
	cors := []kleio.Correlator{
		timewindow.New(cfg.TimeWindow),
		idreference.New(),
		filepath.New(),
	}
	if cfg.Provider != nil && cfg.Provider.Available() {
		cors = append(cors, embed.New(cfg.Provider))
	} else {
		cors = append(cors, search.New(cfg.Store))
	}
	return cors
}

// allSynthesizers returns the canonical synthesizer stack. Plan
// synthesis and orphan synthesis always run; LLM synthesis is purely
// additive when a Provider is available.
func allSynthesizers(cfg Config) []kleio.Synthesizer {
	syns := []kleio.Synthesizer{
		plancluster.New(),
		orphan.New(),
	}
	if cfg.Provider != nil && cfg.Provider.Available() {
		syns = append(syns, llm.New(cfg.Provider))
	}
	return syns
}

func ingesterName(i kleio.Ingester) string       { return i.Name() }
func correlatorName(c kleio.Correlator) string   { return c.Name() }
func synthesizerName(s kleio.Synthesizer) string { return s.Name() }

// filterByName keeps only the entries whose Name() is true in the
// provided map. Generic over the four pipeline element kinds.
func filterByName[T any](in []T, allow map[string]bool, name func(T) string) []T {
	out := in[:0]
	for _, x := range in {
		if allow[name(x)] {
			out = append(out, x)
		}
	}
	return out
}
