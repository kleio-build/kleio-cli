// Package aliases provides anchor expansion for trace/explain/incident.
//
// The `kleio trace "og"` workflow has a long-standing usability gap: the
// human asks for "og" but their corpus uses "opengraph", "og-image",
// "og:image", "twitter:card". The user is forced to remember every
// surface form, defeating the point of a memory tool.
//
// This package solves that with two complementary mechanisms:
//
//  1. **Static aliases** (~/.kleio/aliases.yaml) — user-curated synonyms
//     loaded once at command startup. Zero LLM dependency, deterministic,
//     versionable per project.
//  2. **LLM expansion** (optional, additive) — when an ai.Provider is
//     available, ask it for 3-5 related terms and union with the static
//     set. Cached on disk per anchor so repeat lookups are free.
//
// Engine.Timeline calls Expander.Expand(anchor) once and ORs the results
// into the FTS5 query (one OR group per term, joined with " OR "). The
// alias machinery never deletes terms; it can only widen recall.
package aliases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kleio-build/kleio-cli/internal/ai"
)

// Expander turns a single anchor like "og" into the set of terms an FTS5
// query should match. The original anchor is always included; alias and
// LLM expansions only ever widen the result.
type Expander struct {
	staticMap     map[string][]string
	provider      ai.Provider
	llmCachePath  string
	llmCache      map[string]llmCacheEntry
	llmCacheMu    sync.Mutex
	llmCacheReady bool
	now           func() time.Time
}

type aliasFile struct {
	Aliases map[string][]string `yaml:"aliases"`
}

type llmCacheEntry struct {
	Terms     []string `json:"terms"`
	CachedAt  string   `json:"cached_at"`
	CacheKey  string   `json:"cache_key"`
}

// llmCacheTTL controls how long LLM expansions remain valid before we
// re-ask the model. 30 days is intentionally long — alias expansion is a
// recall hint, not ground truth, so staleness is cheap to absorb.
const llmCacheTTL = 30 * 24 * time.Hour

// New returns an Expander populated from path. When path is empty or
// unreadable the expander still works: it just returns the original
// anchor. provider may be nil (LLM expansion silently skipped).
func New(path string, provider ai.Provider) (*Expander, error) {
	exp := &Expander{
		staticMap: map[string][]string{},
		provider:  provider,
		now:       time.Now,
	}

	if path != "" {
		if err := exp.loadStatic(path); err != nil && !os.IsNotExist(err) {
			return exp, fmt.Errorf("load aliases %q: %w", path, err)
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		exp.llmCachePath = filepath.Join(home, ".kleio", "alias-cache.json")
	}

	return exp, nil
}

// DefaultPath is the conventional location for static aliases. It is the
// same shape as kleio's other config files (~/.kleio/aliases.yaml).
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kleio", "aliases.yaml")
}

// Expand returns the set of search terms for anchor: anchor itself plus
// any static alias matches plus (if a provider is available) cached or
// freshly-fetched LLM expansions. Result is deduplicated and stable.
func (e *Expander) Expand(ctx context.Context, anchor string) []string {
	if e == nil || strings.TrimSpace(anchor) == "" {
		return []string{anchor}
	}

	terms := map[string]bool{}
	terms[strings.TrimSpace(anchor)] = true

	for _, t := range e.lookupStatic(anchor) {
		terms[t] = true
	}

	if e.provider != nil && e.provider.Available() {
		for _, t := range e.llmExpand(ctx, anchor) {
			terms[t] = true
		}
	}

	out := make([]string, 0, len(terms))
	for t := range terms {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func (e *Expander) loadStatic(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file aliasFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}
	for k, vs := range file.Aliases {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		uniq := map[string]bool{}
		for _, v := range vs {
			v = strings.TrimSpace(v)
			if v != "" {
				uniq[v] = true
			}
		}
		out := make([]string, 0, len(uniq))
		for v := range uniq {
			out = append(out, v)
		}
		sort.Strings(out)
		e.staticMap[key] = out
	}
	return nil
}

func (e *Expander) lookupStatic(anchor string) []string {
	key := strings.ToLower(strings.TrimSpace(anchor))
	return e.staticMap[key]
}

func (e *Expander) llmExpand(ctx context.Context, anchor string) []string {
	cacheKey := anchorCacheKey(anchor)
	if cached, ok := e.cacheGet(cacheKey); ok {
		return cached
	}

	prompt := buildExpansionPrompt(anchor)
	resp, err := e.provider.Complete(ctx, prompt)
	if err != nil {
		return nil
	}
	terms := parseExpansionResponse(resp)
	e.cacheSet(cacheKey, terms)
	return terms
}

func buildExpansionPrompt(anchor string) string {
	return fmt.Sprintf(`Suggest 3-5 short alternative phrasings, abbreviations, or related technical terms for the search anchor %q.

Reply with ONE term per line, lowercase, no commentary, no numbering, no bullets.
If you cannot think of any meaningful alternatives, reply with an empty response.`, anchor)
}

// parseExpansionResponse is intentionally forgiving: LLMs love bullets,
// numbering, and quotes. We strip them and keep up to 5 short terms.
func parseExpansionResponse(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		t = strings.TrimLeft(t, "-*0123456789.) ")
		t = strings.Trim(t, "\"'`")
		t = strings.TrimSpace(t)
		if t == "" || len(t) > 60 || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) == 5 {
			break
		}
	}
	return out
}

func anchorCacheKey(anchor string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(anchor))))
	return hex.EncodeToString(sum[:8])
}

func (e *Expander) cacheGet(key string) ([]string, bool) {
	e.llmCacheMu.Lock()
	defer e.llmCacheMu.Unlock()
	e.ensureCacheLoaded()
	entry, ok := e.llmCache[key]
	if !ok {
		return nil, false
	}
	if t, err := time.Parse(time.RFC3339, entry.CachedAt); err == nil {
		if e.now().Sub(t) > llmCacheTTL {
			return nil, false
		}
	}
	out := make([]string, len(entry.Terms))
	copy(out, entry.Terms)
	return out, true
}

func (e *Expander) cacheSet(key string, terms []string) {
	e.llmCacheMu.Lock()
	defer e.llmCacheMu.Unlock()
	e.ensureCacheLoaded()
	e.llmCache[key] = llmCacheEntry{
		Terms:    terms,
		CachedAt: e.now().UTC().Format(time.RFC3339),
		CacheKey: key,
	}
	e.persistCacheLocked()
}

func (e *Expander) ensureCacheLoaded() {
	if e.llmCacheReady {
		return
	}
	e.llmCacheReady = true
	if e.llmCachePath == "" {
		e.llmCache = map[string]llmCacheEntry{}
		return
	}
	data, err := os.ReadFile(e.llmCachePath)
	if err != nil {
		e.llmCache = map[string]llmCacheEntry{}
		return
	}
	cache := map[string]llmCacheEntry{}
	if err := json.Unmarshal(data, &cache); err != nil {
		e.llmCache = map[string]llmCacheEntry{}
		return
	}
	e.llmCache = cache
}

func (e *Expander) persistCacheLocked() {
	if e.llmCachePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(e.llmCachePath), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(e.llmCache, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(e.llmCachePath, data, 0o600)
}
