package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

// CorpusStats describes the shape and density of the local signal corpus
// for a given repo. Used to derive adaptive query/render parameters so
// heuristics scale from 13-commit docs repos to 10k-commit monorepos.
type CorpusStats struct {
	CommitCount    int            `json:"commit_count"`
	EventCount     int            `json:"event_count"`
	TimeSpanDays   float64        `json:"time_span_days"`
	SignalDensity  float64        `json:"signal_density"`
	SourceMix      map[string]int `json:"source_mix"`
	PlanCount      int            `json:"plan_count"`
	MedianGapHours float64        `json:"median_gap_hours"`
	IsSignalRich   bool           `json:"is_signal_rich"`
}

// AdaptiveParams are derived from CorpusStats and control every
// query limit, score threshold, and render cap in the pipeline.
type AdaptiveParams struct {
	TopK            int           `json:"top_k"`
	RecencyHalfLife time.Duration `json:"recency_half_life"`
	ScoreFloor      float64       `json:"score_floor"`
	TimeWindow      time.Duration `json:"time_window"`
	RenderCap       int           `json:"render_cap"`
	RenderDeferCap  int           `json:"render_defer_cap"`
}

var (
	statsCache   sync.Map
	defaultStats = CorpusStats{
		CommitCount:    100,
		EventCount:     100,
		TimeSpanDays:   90,
		SignalDensity:  1.0,
		SourceMix:      map[string]int{},
		MedianGapHours: 4.0,
	}
)

type statsCacheKey struct {
	repo string
	day  string
}

// ComputeCorpusStats queries the store and returns statistics about the
// signal corpus for repoName. Results are cached per (repo, calendar day).
func ComputeCorpusStats(ctx context.Context, store kleio.Store, repoName string) CorpusStats {
	key := statsCacheKey{repo: repoName, day: time.Now().Format("2006-01-02")}
	if cached, ok := statsCache.Load(key); ok {
		return cached.(CorpusStats)
	}

	stats := computeStatsUncached(ctx, store, repoName)
	statsCache.Store(key, stats)
	return stats
}

func computeStatsUncached(ctx context.Context, store kleio.Store, repoName string) CorpusStats {
	if store == nil {
		return defaultStats
	}

	stats := CorpusStats{
		SourceMix: make(map[string]int),
	}

	commits, err := store.QueryCommits(ctx, kleio.CommitFilter{
		RepoName: repoName,
		Limit:    10000,
	})
	if err == nil {
		stats.CommitCount = len(commits)
	}

	events, err := store.ListEvents(ctx, kleio.EventFilter{
		RepoName: repoName,
		Limit:    10000,
	})
	if err == nil {
		stats.EventCount = len(events)
		for _, ev := range events {
			stats.SourceMix[ev.SourceType]++
			if ev.SourceType == "cursor_plan" {
				stats.PlanCount++
			}
		}
	}

	var timestamps []time.Time
	for _, c := range commits {
		if t, err := time.Parse(time.RFC3339, c.CommittedAt); err == nil {
			timestamps = append(timestamps, t)
		}
	}
	for _, ev := range events {
		if t, err := time.Parse(time.RFC3339, ev.CreatedAt); err == nil {
			timestamps = append(timestamps, t)
		}
	}

	if len(timestamps) > 1 {
		sort.Slice(timestamps, func(i, j int) bool { return timestamps[i].Before(timestamps[j]) })
		span := timestamps[len(timestamps)-1].Sub(timestamps[0])
		stats.TimeSpanDays = span.Hours() / 24.0
		if stats.TimeSpanDays < 0.01 {
			stats.TimeSpanDays = 0.01
		}

		var gaps []float64
		for i := 1; i < len(timestamps); i++ {
			gap := timestamps[i].Sub(timestamps[i-1]).Hours()
			if gap > 0 {
				gaps = append(gaps, gap)
			}
		}
		if len(gaps) > 0 {
			sort.Float64s(gaps)
			stats.MedianGapHours = gaps[len(gaps)/2]
		}
	} else if len(timestamps) == 1 {
		stats.TimeSpanDays = 1.0
		stats.MedianGapHours = 4.0
	} else {
		stats.TimeSpanDays = 1.0
		stats.MedianGapHours = 4.0
	}

	denom := stats.CommitCount
	if denom < 1 {
		denom = 1
	}
	stats.SignalDensity = float64(stats.EventCount) / float64(denom)
	stats.IsSignalRich = stats.PlanCount > 0 && stats.SignalDensity > 0.5

	return stats
}

// DeriveParams converts corpus statistics into adaptive pipeline parameters.
func DeriveParams(stats CorpusStats) AdaptiveParams {
	topK := clampInt(15, int(float64(stats.EventCount)*0.15), 200)

	halfLifeDays := math.Max(7.0, stats.TimeSpanDays/10.0)
	halfLife := time.Duration(halfLifeDays*24) * time.Hour

	windowHours := math.Max(0.25, stats.MedianGapHours*3.0)
	timeWindow := time.Duration(windowHours * float64(time.Hour))
	if timeWindow < 15*time.Minute {
		timeWindow = 15 * time.Minute
	}

	renderCap := clampInt(5, topK/2, 20)
	renderDeferCap := clampInt(3, topK/4, 10)

	return AdaptiveParams{
		TopK:            topK,
		RecencyHalfLife: halfLife,
		ScoreFloor:      0.05,
		TimeWindow:      timeWindow,
		RenderCap:       renderCap,
		RenderDeferCap:  renderDeferCap,
	}
}

// DefaultParams returns safe defaults for when CorpusStats are unavailable.
func DefaultParams() AdaptiveParams {
	return DeriveParams(defaultStats)
}

func (s CorpusStats) String() string {
	return fmt.Sprintf(
		"commits=%d events=%d span=%.0fd density=%.2f plans=%d median_gap=%.1fh signal_rich=%v",
		s.CommitCount, s.EventCount, s.TimeSpanDays, s.SignalDensity,
		s.PlanCount, s.MedianGapHours, s.IsSignalRich,
	)
}

func (p AdaptiveParams) String() string {
	return fmt.Sprintf(
		"top_k=%d half_life=%s floor=%.2f window=%s render_cap=%d defer_cap=%d",
		p.TopK, p.RecencyHalfLife, p.ScoreFloor, p.TimeWindow, p.RenderCap, p.RenderDeferCap,
	)
}

func clampInt(min, val, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
