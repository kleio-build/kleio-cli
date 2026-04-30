package engine

import (
	"math"
	"sort"
	"strings"
	"time"
)

// ScoredItem wraps any result with a composite relevance score.
type ScoredItem[T any] struct {
	Item  T
	Score float64
}

// RankConfig tunes the weight of each scoring dimension.
type RankConfig struct {
	RecencyWeight    float64
	KeywordWeight    float64
	FileOverlap      float64
	IdentifierWeight float64
}

// DefaultRankConfig returns balanced weights.
func DefaultRankConfig() RankConfig {
	return RankConfig{
		RecencyWeight:    0.35,
		KeywordWeight:    0.30,
		FileOverlap:      0.20,
		IdentifierWeight: 0.15,
	}
}

// recencyScore returns a value in [0,1] decaying exponentially with age.
// Half-life is 7 days.
func recencyScore(ts string) float64 {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return 0
	}
	hours := time.Since(t).Hours()
	if hours < 0 {
		hours = 0
	}
	halfLife := 7.0 * 24
	return math.Exp(-0.693 * hours / halfLife)
}

// keywordScore returns the fraction of query keywords found in text.
func keywordScore(text, query string) float64 {
	lower := strings.ToLower(text)
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return 0
	}
	hits := 0
	for _, w := range words {
		if strings.Contains(lower, w) {
			hits++
		}
	}
	return float64(hits) / float64(len(words))
}

// fileOverlapScore computes Jaccard similarity between two file sets.
func fileOverlapScore(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := make(map[string]bool, len(a))
	for _, f := range a {
		set[f] = true
	}
	inter := 0
	union := len(set)
	for _, f := range b {
		if set[f] {
			inter++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// SortByScore sorts scored items in descending order.
func SortByScore[T any](items []ScoredItem[T]) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})
}
