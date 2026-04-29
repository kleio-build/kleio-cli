package gitreader

// Coverage contract:
//
// Format (text):
// - standup: shows tasks, tickets, effort, commits
// - pr: shows PR summary with tickets and changes
// - week: groups tasks by day
// - empty result shows "No activity found."
//
// Format (json):
// - produces valid JSON
// - includes task_count, commit_count, tasks array
// - tasks include summary, branch, tickets, effort_minutes

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleResult() *ScanResult {
	now := time.Date(2026, 4, 28, 14, 0, 0, 0, time.UTC)
	return &ScanResult{
		Tasks: []Task{
			{
				Summary: "feat: add auth flow",
				Branch:  "feature/AUTH-42",
				Tickets: []string{"AUTH-42"},
				Effort:  45 * time.Minute,
				StartAt: now.Add(-2 * time.Hour),
				EndAt:   now.Add(-1 * time.Hour),
				Commits: []Commit{
					{Hash: "abc12345", Message: "feat: add login endpoint", Author: "dev", Timestamp: now.Add(-2 * time.Hour)},
					{Hash: "def67890", Message: "feat: add session handling", Author: "dev", Timestamp: now.Add(-1 * time.Hour)},
				},
			},
			{
				Summary: "fix: null pointer in signup",
				Branch:  "fix/signup",
				Effort:  15 * time.Minute,
				StartAt: now,
				EndAt:   now,
				Commits: []Commit{
					{Hash: "ghi11111", Message: "fix: null pointer in signup", Author: "dev", Timestamp: now},
				},
			},
		},
		Commits: []Commit{
			{Hash: "abc12345"}, {Hash: "def67890"}, {Hash: "ghi11111"},
		},
	}
}

func TestFormat_StandupText(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	result := sampleResult()

	// Act
	err := Format(&buf, result, FormatText, ViewStandup)

	// Assert
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "Standup")
	assert.Contains(t, out, "2 tasks")
	assert.Contains(t, out, "AUTH-42")
	assert.Contains(t, out, "feat: add auth flow")
	assert.Contains(t, out, "fix: null pointer in signup")
}

func TestFormat_PRText(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	result := sampleResult()

	// Act
	err := Format(&buf, result, FormatText, ViewPR)

	// Assert
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "PR Summary")
	assert.Contains(t, out, "AUTH-42")
	assert.Contains(t, out, "Changes:")
}

func TestFormat_WeekText(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	result := sampleResult()

	// Act
	err := Format(&buf, result, FormatText, ViewWeek)

	// Assert
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "Weekly Summary")
	assert.Contains(t, out, "Tue Apr 28")
}

func TestFormat_EmptyResult(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	result := &ScanResult{}

	// Act
	err := Format(&buf, result, FormatText, ViewStandup)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No activity found.")
}

func TestFormat_JSONOutput(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	result := sampleResult()

	// Act
	err := Format(&buf, result, FormatJSON, ViewStandup)

	// Assert
	require.NoError(t, err)

	var parsed jsonOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))

	assert.Equal(t, "standup", parsed.View)
	assert.Equal(t, 2, parsed.TaskCount)
	assert.Equal(t, 3, parsed.CommitCount)
	require.Len(t, parsed.Tasks, 2)
	assert.Equal(t, "feat: add auth flow", parsed.Tasks[0].Summary)
	assert.Equal(t, 45, parsed.Tasks[0].EffortMins)
	assert.Equal(t, []string{"AUTH-42"}, parsed.Tasks[0].Tickets)
}
