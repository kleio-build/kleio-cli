package gitreader

import "time"

type ScanOptions struct {
	RepoPath    string
	Since       time.Time
	Author      string
	Branch      string
	NoiseFilter bool
}

// Scan is the high-level entry point that combines walking, filtering,
// grouping, ticket extraction, and effort estimation into a single result.
func Scan(opts ScanOptions) (*ScanResult, error) {
	walkOpts := WalkOptions{
		RepoPath: opts.RepoPath,
		Since:    opts.Since,
		Author:   opts.Author,
		Branch:   opts.Branch,
	}

	commits, err := Walk(walkOpts)
	if err != nil {
		return nil, err
	}

	noiseCfg := DefaultNoiseConfig()
	noiseCfg.Enabled = opts.NoiseFilter
	filter := NewNoiseFilter(noiseCfg)
	filtered := filter.Apply(commits)

	grouper := NewTaskGrouper()
	tasks := grouper.Group(filtered)

	extractor := NewTicketExtractor()
	estimator := NewEffortEstimator()
	for i := range tasks {
		for _, c := range tasks[i].Commits {
			extracted := extractor.Extract(tasks[i].Branch, c.Message)
			for _, t := range extracted {
				if !containsStr(tasks[i].Tickets, t) {
					tasks[i].Tickets = append(tasks[i].Tickets, t)
				}
			}
		}
		tasks[i].Effort = estimator.Estimate(tasks[i].Commits)
	}

	return &ScanResult{
		Tasks:   tasks,
		Commits: filtered,
		Since:   opts.Since,
		Author:  opts.Author,
		Branch:  opts.Branch,
	}, nil
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
