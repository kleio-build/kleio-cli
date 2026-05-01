// Package search implements a kleio.Correlator that uses the Store's
// Search() method to discover semantic similarities between RawSignals.
//
// Why this design:
//
//   - Store.Search abstracts over text search (FTS5 in localdb) and
//     semantic search (embeddings in cloud). The same Correlator works
//     in both modes; behaviour is "best available" without ever
//     requiring an LLM at the CLI layer.
//
//   - This package is what lets us promise "semantic correlation
//     without ollama". Users get a meaningful semantic-ish layer on
//     day one (FTS5 fuzzy matches identifiers, code symbols, plan
//     section names); cloud users get true embedding similarity for
//     free; LLM-equipped users (Phase 3.5) get embeddings via
//     ai.Provider.Embed.
//
// Limitations: FTS5 only matches token overlap, so "og" won't pull
// "opengraph" without alias expansion (Phase 5.4 covers that). The
// EmbedCorrelator (Phase 3.5) replaces SearchCorrelator transparently
// when an LLM is available so the upper bound is significantly higher.
package search

import (
	"context"
	"fmt"
	"sort"
	"strings"

	kleio "github.com/kleio-build/kleio-core"
)

// Correlator implements kleio.Correlator using Store.Search.
type Correlator struct {
	// Store is the search backend. When nil, Correlate is a no-op
	// (returns no clusters and no error). This intentional fallback
	// keeps the pipeline runnable without a configured Store -- the
	// other correlators still produce useful clusters.
	Store kleio.Store

	// QueryLimit caps the number of search results per signal. Default
	// 8 keeps total query count manageable on FTS5 (~O(N*8) queries
	// for N signals; for N~1000 that's ~8000 fast SELECTs).
	QueryLimit int

	// MinScore filters low-quality matches (Search returns rank scores
	// but a low rank can mean "barely matched a single token"). 0.1
	// is a safe default for FTS5; embedding backends will return
	// higher rank ranges already and the threshold barely matters.
	MinScore float64

	// MinClusterSize gates emission. Default 2.
	MinClusterSize int

	// QueryTokenLimit caps how many tokens of a signal's Content are
	// used as the search query. FTS5 chokes on very long queries.
	QueryTokenLimit int
}

func New(store kleio.Store) *Correlator {
	return &Correlator{
		Store:           store,
		QueryLimit:      8,
		MinScore:        0.1,
		MinClusterSize:  2,
		QueryTokenLimit: 24,
	}
}

func (c *Correlator) Name() string { return "search" }

// Correlate uses Store.Search to find related signals for each input.
// It builds an undirected graph of "search-similar" pairs, runs
// connected components on the graph, and emits one Cluster per
// component with at least MinClusterSize members.
func (c *Correlator) Correlate(ctx context.Context, signals []kleio.RawSignal) ([]kleio.Cluster, error) {
	if c.Store == nil || len(signals) == 0 {
		return nil, nil
	}
	limit := c.QueryLimit
	if limit <= 0 {
		limit = 8
	}
	minSize := c.MinClusterSize
	if minSize <= 0 {
		minSize = 2
	}
	tokenLimit := c.QueryTokenLimit
	if tokenLimit <= 0 {
		tokenLimit = 24
	}

	signalsByKey := map[string]kleio.RawSignal{}
	signalKeys := make([]string, 0, len(signals))
	for _, s := range signals {
		key := signalKey(s)
		signalsByKey[key] = s
		signalKeys = append(signalKeys, key)
	}

	var edges []searchEdge
	for _, s := range signals {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		query := buildQuery(s.Content, tokenLimit)
		if query == "" {
			continue
		}
		results, err := c.Store.Search(ctx, query, kleio.SearchOpts{Limit: limit})
		if err != nil {
			continue
		}
		fromKey := signalKey(s)
		for _, r := range results {
			if r.Score < c.MinScore {
				continue
			}
			toKey := r.ID
			if toKey == "" || toKey == fromKey {
				continue
			}
			if _, ok := signalsByKey[toKey]; !ok {
				continue
			}
			edges = append(edges, searchEdge{from: fromKey, to: toKey, score: r.Score})
		}
	}

	sort.Strings(signalKeys)
	uf := newUnionFind(signalKeys)
	for _, e := range edges {
		uf.union(e.from, e.to)
	}

	roots := map[string][]kleio.RawSignal{}
	for _, k := range signalKeys {
		root := uf.find(k)
		roots[root] = append(roots[root], signalsByKey[k])
	}

	rootKeys := make([]string, 0, len(roots))
	for k := range roots {
		rootKeys = append(rootKeys, k)
	}
	sort.Strings(rootKeys)

	var clusters []kleio.Cluster
	for _, root := range rootKeys {
		members := roots[root]
		if len(members) < minSize {
			continue
		}
		sort.SliceStable(members, func(i, j int) bool {
			return members[i].Timestamp.Before(members[j].Timestamp)
		})
		anchor := pickAnchor(members)
		links := buildLinks(anchor, members, edges)
		clusters = append(clusters, kleio.Cluster{
			AnchorID:   anchor,
			AnchorType: anchorType(members, anchor),
			Members:    members,
			Links:      links,
			Confidence: 0.55,
			Provenance: []string{c.Name()},
		})
	}
	return clusters, nil
}

// buildQuery returns a sanitised, length-capped FTS5 query string. We
// strip non-word punctuation so " I'll " doesn't trip the FTS tokenizer.
func buildQuery(content string, tokenLimit int) string {
	if content == "" {
		return ""
	}
	var b strings.Builder
	tokens := 0
	for _, raw := range strings.FieldsFunc(content, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == ',' || r == '.' || r == '!' ||
			r == '?' || r == '"' || r == '\'' || r == '(' || r == ')' ||
			r == '[' || r == ']' || r == '{' || r == '}' || r == ':' || r == ';'
	}) {
		t := strings.TrimSpace(raw)
		if len(t) < 3 {
			continue
		}
		if tokens > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(t)
		tokens++
		if tokens >= tokenLimit {
			break
		}
	}
	return b.String()
}

// searchEdge is a single search-similarity hit between two signal
// keys. Promoted out of Correlate so buildLinks can refer to its type.
type searchEdge struct {
	from, to string
	score    float64
}

func buildLinks(anchor string, members []kleio.RawSignal, edges []searchEdge) []kleio.ClusterLink {
	out := make([]kleio.ClusterLink, 0, len(members))
	for _, m := range members {
		if signalKey(m) == anchor {
			continue
		}
		score := 0.55
		for _, e := range edges {
			if (e.from == anchor && e.to == signalKey(m)) || (e.to == anchor && e.from == signalKey(m)) {
				if e.score > score {
					score = e.score
				}
			}
		}
		out = append(out, kleio.ClusterLink{
			From:       anchor,
			To:         signalKey(m),
			LinkType:   kleio.LinkTypeKeywordMatch,
			Confidence: score,
			Reason:     fmt.Sprintf("search_score=%.2f", score),
		})
	}
	return out
}

func pickAnchor(members []kleio.RawSignal) string {
	bestScore := -1
	best := ""
	for _, m := range members {
		score := 0
		switch m.SourceType {
		case "cursor_plan":
			score += 100
		case kleio.SourceTypeLocalGit:
			score += 50
		case kleio.SourceTypeCursorTranscript:
			score += 10
		}
		if m.SourceOffset == "umbrella" {
			score += 50
		}
		if score > bestScore {
			bestScore = score
			best = signalKey(m)
		}
	}
	if best == "" && len(members) > 0 {
		best = signalKey(members[0])
	}
	return best
}

func anchorType(members []kleio.RawSignal, anchor string) string {
	for _, m := range members {
		if signalKey(m) == anchor {
			return m.SourceType
		}
	}
	if len(members) > 0 {
		return members[0].SourceType
	}
	return ""
}

func signalKey(s kleio.RawSignal) string {
	if s.SourceID != "" {
		return s.SourceID
	}
	return s.SourceType + ":" + s.SourceOffset
}

// unionFind supports the connected-components calculation that turns
// pairwise search hits into clusters.
type unionFind struct {
	parent map[string]string
}

func newUnionFind(keys []string) *unionFind {
	uf := &unionFind{parent: make(map[string]string, len(keys))}
	for _, k := range keys {
		uf.parent[k] = k
	}
	return uf
}

func (u *unionFind) find(x string) string {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b string) {
	ra := u.find(a)
	rb := u.find(b)
	if ra == rb {
		return
	}
	if ra < rb {
		u.parent[rb] = ra
	} else {
		u.parent[ra] = rb
	}
}
