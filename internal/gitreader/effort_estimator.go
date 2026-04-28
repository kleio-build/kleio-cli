package gitreader

import (
	"sort"
	"time"
)

const (
	gapThreshold = 2 * time.Hour
	minEffort    = 15 * time.Minute
)

type EffortEstimator struct{}

func NewEffortEstimator() *EffortEstimator {
	return &EffortEstimator{}
}

// Estimate returns the estimated working time for a set of commits.
// Gaps under 2h count as work time; longer gaps are capped at the threshold.
// A single commit returns a minimum of 15 minutes.
func (e *EffortEstimator) Estimate(commits []Commit) time.Duration {
	if len(commits) == 0 {
		return 0
	}
	if len(commits) == 1 {
		return minEffort
	}

	sorted := make([]Commit, len(commits))
	copy(sorted, commits)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	var total time.Duration
	for i := 1; i < len(sorted); i++ {
		gap := sorted[i].Timestamp.Sub(sorted[i-1].Timestamp)
		if gap < 0 {
			gap = -gap
		}
		if gap > gapThreshold {
			gap = gapThreshold
		}
		total += gap
	}

	if total < minEffort {
		return minEffort
	}
	return total
}
