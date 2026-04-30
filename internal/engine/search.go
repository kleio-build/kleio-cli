package engine

import (
	"context"

	kleio "github.com/kleio-build/kleio-core"
)

// SearchResult wraps a store search result with engine scoring.
type SearchResult struct {
	kleio.SearchResult
	EngineScore float64
}

// Search runs a ranked search across all local data. When a BYOK LLM is
// configured, embeddings enhance ranking; otherwise plain text search with
// recency/keyword scoring is used.
func (e *Engine) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	raw, err := e.store.Search(ctx, query, kleio.SearchOpts{Limit: limit * 2})
	if err != nil {
		return nil, err
	}

	cfg := DefaultRankConfig()
	results := make([]SearchResult, 0, len(raw))
	for _, r := range raw {
		score := cfg.RecencyWeight*recencyScore(r.CreatedAt) +
			cfg.KeywordWeight*keywordScore(r.Content, query) +
			cfg.FileOverlap*keywordScore(r.FilePath, query)

		results = append(results, SearchResult{
			SearchResult: r,
			EngineScore:  score,
		})
	}

	scored := make([]ScoredItem[SearchResult], len(results))
	for i, r := range results {
		scored[i] = ScoredItem[SearchResult]{Item: r, Score: r.EngineScore}
	}
	SortByScore(scored)

	out := make([]SearchResult, 0, limit)
	for i, s := range scored {
		if i >= limit {
			break
		}
		out = append(out, s.Item)
	}
	return out, nil
}
