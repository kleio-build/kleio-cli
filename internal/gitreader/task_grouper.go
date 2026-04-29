package gitreader

import (
	"fmt"
	"strings"
)

type TaskGrouper struct{}

func NewTaskGrouper() *TaskGrouper {
	return &TaskGrouper{}
}

func (g *TaskGrouper) Group(commits []Commit) []Task {
	if len(commits) == 0 {
		return nil
	}

	var tasks []Task
	var current []Commit

	for i, c := range commits {
		if i == 0 {
			current = []Commit{c}
			continue
		}
		if c.Branch == commits[i-1].Branch {
			current = append(current, c)
		} else {
			tasks = append(tasks, buildTask(current))
			current = []Commit{c}
		}
	}
	if len(current) > 0 {
		tasks = append(tasks, buildTask(current))
	}

	return tasks
}

func buildTask(commits []Commit) Task {
	t := Task{
		Branch:  commits[0].Branch,
		Commits: commits,
		StartAt: commits[0].Timestamp,
		EndAt:   commits[len(commits)-1].Timestamp,
	}
	if t.StartAt.After(t.EndAt) {
		t.StartAt, t.EndAt = t.EndAt, t.StartAt
	}
	t.Summary = deriveSummary(commits)
	return t
}

func deriveSummary(commits []Commit) string {
	if len(commits) == 1 {
		return firstLine(commits[0].Message)
	}
	var parts []string
	for _, c := range commits {
		line := firstLine(c.Message)
		if line != "" {
			parts = append(parts, line)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) <= 3 {
		return strings.Join(parts, "; ")
	}
	return strings.Join(parts[:3], "; ") + " (+" + itoa(len(parts)-3) + " more)"
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return strings.TrimSpace(s)
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
