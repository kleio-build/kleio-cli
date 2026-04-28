package gitreader

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type FormatMode string

const (
	FormatText FormatMode = "text"
	FormatJSON FormatMode = "json"
)

type ScanView string

const (
	ViewStandup ScanView = "standup"
	ViewPR      ScanView = "pr"
	ViewWeek    ScanView = "week"
)

// Format writes the scan result to w in the given mode and view.
func Format(w io.Writer, result *ScanResult, mode FormatMode, view ScanView) error {
	if mode == FormatJSON {
		return formatJSON(w, result, view)
	}
	return formatText(w, result, view)
}

func formatJSON(w io.Writer, result *ScanResult, view ScanView) error {
	out := jsonOutput{
		View:       string(view),
		TaskCount:  len(result.Tasks),
		CommitCount: len(result.Commits),
		Tasks:      make([]jsonTask, 0, len(result.Tasks)),
	}
	if !result.Since.IsZero() {
		s := result.Since.Format(time.RFC3339)
		out.Since = &s
	}

	for _, t := range result.Tasks {
		jt := jsonTask{
			Summary:     t.Summary,
			Branch:      t.Branch,
			Tickets:     t.Tickets,
			CommitCount: len(t.Commits),
			EffortMins:  int(t.Effort.Minutes()),
		}
		if !t.StartAt.IsZero() {
			s := t.StartAt.Format(time.RFC3339)
			jt.StartAt = &s
		}
		if !t.EndAt.IsZero() {
			s := t.EndAt.Format(time.RFC3339)
			jt.EndAt = &s
		}
		for _, c := range t.Commits {
			jt.Commits = append(jt.Commits, jsonCommit{
				Hash:    c.Hash[:minInt(8, len(c.Hash))],
				Message: firstLine(c.Message),
				Author:  c.Author,
				Time:    c.Timestamp.Format(time.RFC3339),
				Files:   c.Files,
			})
		}
		out.Tasks = append(out.Tasks, jt)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func formatText(w io.Writer, result *ScanResult, view ScanView) error {
	if len(result.Tasks) == 0 {
		fmt.Fprintln(w, "No activity found.")
		return nil
	}

	switch view {
	case ViewWeek:
		return formatWeekText(w, result)
	case ViewPR:
		return formatPRText(w, result)
	default:
		return formatStandupText(w, result)
	}
}

func formatStandupText(w io.Writer, result *ScanResult) error {
	fmt.Fprintf(w, "Standup — %d tasks, %d commits\n", len(result.Tasks), len(result.Commits))
	fmt.Fprintln(w, strings.Repeat("─", 50))

	for _, t := range result.Tasks {
		ticketStr := ""
		if len(t.Tickets) > 0 {
			ticketStr = " [" + strings.Join(t.Tickets, ", ") + "]"
		}
		effortStr := formatDuration(t.Effort)
		fmt.Fprintf(w, "\n• %s%s (~%s)\n", t.Summary, ticketStr, effortStr)
		for _, c := range t.Commits {
			fmt.Fprintf(w, "  %s %s\n", c.Hash[:minInt(8, len(c.Hash))], firstLine(c.Message))
		}
	}
	return nil
}

func formatPRText(w io.Writer, result *ScanResult) error {
	fmt.Fprintf(w, "PR Summary — %d commits\n", len(result.Commits))
	fmt.Fprintln(w, strings.Repeat("─", 50))

	var allTickets []string
	for _, t := range result.Tasks {
		allTickets = append(allTickets, t.Tickets...)
	}
	if len(allTickets) > 0 {
		fmt.Fprintf(w, "Tickets: %s\n", strings.Join(dedup(allTickets), ", "))
	}

	fmt.Fprintln(w, "\nChanges:")
	for _, t := range result.Tasks {
		fmt.Fprintf(w, "  • %s (%d commits)\n", t.Summary, len(t.Commits))
	}
	return nil
}

func formatWeekText(w io.Writer, result *ScanResult) error {
	fmt.Fprintf(w, "Weekly Summary — %d tasks, %d commits\n", len(result.Tasks), len(result.Commits))
	fmt.Fprintln(w, strings.Repeat("─", 50))

	dayGroups := make(map[string][]Task)
	var dayOrder []string
	for _, t := range result.Tasks {
		ts := t.StartAt
		if ts.IsZero() && len(t.Commits) > 0 {
			ts = t.Commits[0].Timestamp
		}
		day := ts.Format("Mon Jan 2")
		if _, exists := dayGroups[day]; !exists {
			dayOrder = append(dayOrder, day)
		}
		dayGroups[day] = append(dayGroups[day], t)
	}

	for _, day := range dayOrder {
		fmt.Fprintf(w, "\n%s\n", day)
		for _, t := range dayGroups[day] {
			ticketStr := ""
			if len(t.Tickets) > 0 {
				ticketStr = " [" + strings.Join(t.Tickets, ", ") + "]"
			}
			fmt.Fprintf(w, "  • %s%s (%d commits)\n", t.Summary, ticketStr, len(t.Commits))
		}
	}
	return nil
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func dedup(ss []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type jsonOutput struct {
	View        string     `json:"view"`
	Since       *string    `json:"since,omitempty"`
	TaskCount   int        `json:"task_count"`
	CommitCount int        `json:"commit_count"`
	Tasks       []jsonTask `json:"tasks"`
}

type jsonTask struct {
	Summary     string       `json:"summary"`
	Branch      string       `json:"branch"`
	Tickets     []string     `json:"tickets,omitempty"`
	CommitCount int          `json:"commit_count"`
	EffortMins  int          `json:"effort_minutes"`
	StartAt     *string      `json:"start_at,omitempty"`
	EndAt       *string      `json:"end_at,omitempty"`
	Commits     []jsonCommit `json:"commits"`
}

type jsonCommit struct {
	Hash    string   `json:"hash"`
	Message string   `json:"message"`
	Author  string   `json:"author"`
	Time    string   `json:"time"`
	Files   []string `json:"files,omitempty"`
}
