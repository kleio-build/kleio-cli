// Package embed implements a kleio.Correlator that uses an
// ai.Provider's Embed method to compute pairwise cosine similarity
// across RawSignals.
//
// Promotion model: this correlator is the strict superset of
// SearchCorrelator. When ai.AutoDetect returns an LLM-equipped
// Provider, the pipeline swaps SearchCorrelator out for EmbedCorrelator
// transparently. When no LLM is available, SearchCorrelator continues
// to handle semantic correlation via Store.Search (FTS5 locally,
// embeddings in cloud) -- the user never sees a degraded experience.
//
// Cost & determinism: embedding every signal costs O(N) Embed calls,
// which is acceptable for ollama on commodity hardware (~30ms each)
// and trivially cheap for hosted APIs. We cache nothing because
// signals change every ingest run; if performance becomes a problem,
// a future iteration can persist embeddings keyed by signal SourceID.
package embed

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/kleio-build/kleio-cli/internal/ai"
	kleio "github.com/kleio-build/kleio-core"
)

// Correlator wraps an ai.Provider with embedding-based clustering.
type Correlator struct {
	// Provider is the embedding backend. When nil or
	// !Provider.Available(), Correlate is a no-op (callers should fall
	// back to SearchCorrelator instead).
	Provider ai.Provider

	// Threshold is the minimum cosine similarity for two signals to be
	// considered related. Default 0.78 is calibrated from llama3.2's
	// embed quality: 0.7 has too many false positives ("any two
	// English sentences"), 0.85 has too many false negatives. Override
	// when using a different embed model.
	Threshold float64

	// MinClusterSize gates emission. Default 2.
	MinClusterSize int

	// ContentByteLimit caps the input to Embed per signal. Embedding
	// models have token limits (~512 for many local models) so we
	// truncate aggressively. Plans and transcripts can be long.
	ContentByteLimit int
}

func New(provider ai.Provider) *Correlator {
	return &Correlator{
		Provider:         provider,
		Threshold:        0.78,
		MinClusterSize:   2,
		ContentByteLimit: 1500,
	}
}

func (c *Correlator) Name() string { return "embed" }

// Available reports whether the correlator can run. The pipeline
// inspects this before deciding whether to register Correlator or fall
// back to SearchCorrelator.
func (c *Correlator) Available() bool {
	return c.Provider != nil && c.Provider.Available()
}

// Correlate computes one embedding per signal, then emits clusters
// based on connected components in the similarity graph (edges
// emitted whenever cosine(a, b) >= Threshold).
//
// Performance notes:
//   - O(N) Embed calls (N = len(signals)).
//   - O(N^2) cosine comparisons. For N=1000 that's 1M cheap float
//     operations; we don't expect this to be the bottleneck before
//     N=10k.
func (c *Correlator) Correlate(ctx context.Context, signals []kleio.RawSignal) ([]kleio.Cluster, error) {
	if !c.Available() || len(signals) == 0 {
		return nil, nil
	}
	thresh := c.Threshold
	if thresh <= 0 {
		thresh = 0.78
	}
	minSize := c.MinClusterSize
	if minSize <= 0 {
		minSize = 2
	}
	byteLimit := c.ContentByteLimit
	if byteLimit <= 0 {
		byteLimit = 1500
	}

	embeddings := make([][]float64, len(signals))
	for i, s := range signals {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		text := truncate(s.Content, byteLimit)
		if text == "" {
			continue
		}
		vec, err := c.Provider.Embed(ctx, text)
		if err != nil {
			continue
		}
		embeddings[i] = vec
	}

	type edge struct {
		i, j int
		sim  float64
	}
	var edges []edge
	for i := 0; i < len(signals); i++ {
		if embeddings[i] == nil {
			continue
		}
		for j := i + 1; j < len(signals); j++ {
			if embeddings[j] == nil {
				continue
			}
			sim, ok := cosine(embeddings[i], embeddings[j])
			if !ok || sim < thresh {
				continue
			}
			if signals[i].RepoName != "" && signals[j].RepoName != "" && signals[i].RepoName != signals[j].RepoName {
				continue
			}
			edges = append(edges, edge{i: i, j: j, sim: sim})
		}
	}

	uf := newUnionFind(len(signals))
	for _, e := range edges {
		uf.union(e.i, e.j)
	}

	roots := map[int][]int{}
	for i := range signals {
		root := uf.find(i)
		roots[root] = append(roots[root], i)
	}

	rootKeys := make([]int, 0, len(roots))
	for k := range roots {
		rootKeys = append(rootKeys, k)
	}
	sort.Ints(rootKeys)

	var clusters []kleio.Cluster
	for _, root := range rootKeys {
		idxs := roots[root]
		if len(idxs) < minSize {
			continue
		}
		members := make([]kleio.RawSignal, len(idxs))
		for k, ix := range idxs {
			members[k] = signals[ix]
		}
		sort.SliceStable(members, func(a, b int) bool {
			return members[a].Timestamp.Before(members[b].Timestamp)
		})
		anchor := pickAnchor(members)
		links := make([]kleio.ClusterLink, 0, len(members))
		for _, m := range members {
			if signalKey(m) == anchor {
				continue
			}
			links = append(links, kleio.ClusterLink{
				From:       anchor,
				To:         signalKey(m),
				LinkType:   kleio.LinkTypeRelatedTo,
				Confidence: 0.7,
				Reason:     fmt.Sprintf("embed_cosine>=%g", thresh),
			})
		}
		clusters = append(clusters, kleio.Cluster{
			AnchorID:   anchor,
			AnchorType: anchorType(members, anchor),
			Members:    members,
			Links:      links,
			Confidence: 0.7,
			Provenance: []string{c.Name()},
		})
	}
	return clusters, nil
}

// cosine returns the cosine similarity of two equal-length vectors.
// Returns (0, false) when dimensions differ or one vector is zero.
func cosine(a, b []float64) (float64, bool) {
	if len(a) != len(b) || len(a) == 0 {
		return 0, false
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0, false
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb)), true
}

func truncate(s string, byteLimit int) string {
	if len(s) <= byteLimit {
		return s
	}
	return s[:byteLimit]
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

type unionFind struct{ parent []int }

func newUnionFind(n int) *unionFind {
	uf := &unionFind{parent: make([]int, n)}
	for i := range uf.parent {
		uf.parent[i] = i
	}
	return uf
}

func (u *unionFind) find(x int) int {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b int) {
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
