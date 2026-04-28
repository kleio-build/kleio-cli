package gitreader

// Coverage contract:
//
// CommitWalker:
// - walks commits from HEAD on default branch
// - respects --since time filter
// - respects --author email filter
// - walks commits on a specific branch (for PR mode)
// - returns empty slice for empty repo
//
// NoiseFilter:
// - excludes merge commits
// - excludes lockfile-only commits (package-lock.json, go.sum, yarn.lock, etc.)
// - excludes commits with empty messages
// - keeps meaningful commits
// - configurable: --no-filter-noise disables
//
// TaskGrouper:
// - groups consecutive commits on same branch into one task
// - single commit = single task
// - interleaved branches = separate tasks
// - derives task summary from grouped commit messages
//
// TicketExtractor:
// - extracts JIRA-style IDs from branch names (feature/AUTH-42-login)
// - extracts GitHub issue refs (#15) from commit messages
// - extracts KL-N refs from commit messages and branch names
// - returns empty when no tickets found
//
// EffortEstimator:
// - gaps under 2h between commits = work time
// - gaps over 2h are capped (break)
// - single commit = minimum effort (15 min)

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NoiseFilter tests ---

func TestNoiseFilter_ExcludesMergeCommits(t *testing.T) {
	// Arrange
	commits := []Commit{
		{Message: "feat: add login", IsMerge: false},
		{Message: "Merge branch 'main' into feature/login", IsMerge: true},
	}
	filter := NewNoiseFilter(DefaultNoiseConfig())

	// Act
	filtered := filter.Apply(commits)

	// Assert
	assert.Len(t, filtered, 1)
	assert.Equal(t, "feat: add login", filtered[0].Message)
}

func TestNoiseFilter_ExcludesLockfileOnlyCommits(t *testing.T) {
	// Arrange
	commits := []Commit{
		{Message: "chore: update deps", Files: []string{"package-lock.json"}},
		{Message: "feat: add auth", Files: []string{"auth.go", "auth_test.go"}},
		{Message: "chore: update go.sum", Files: []string{"go.sum"}},
		{Message: "chore: update yarn lock", Files: []string{"yarn.lock"}},
		{Message: "chore: mixed", Files: []string{"go.sum", "main.go"}},
	}
	filter := NewNoiseFilter(DefaultNoiseConfig())

	// Act
	filtered := filter.Apply(commits)

	// Assert
	assert.Len(t, filtered, 2)
	assert.Equal(t, "feat: add auth", filtered[0].Message)
	assert.Equal(t, "chore: mixed", filtered[1].Message)
}

func TestNoiseFilter_ExcludesEmptyMessages(t *testing.T) {
	// Arrange
	commits := []Commit{
		{Message: ""},
		{Message: "   "},
		{Message: "feat: real commit"},
	}
	filter := NewNoiseFilter(DefaultNoiseConfig())

	// Act
	filtered := filter.Apply(commits)

	// Assert
	assert.Len(t, filtered, 1)
	assert.Equal(t, "feat: real commit", filtered[0].Message)
}

func TestNoiseFilter_DisabledKeepsAll(t *testing.T) {
	// Arrange
	commits := []Commit{
		{Message: "Merge branch 'main'", IsMerge: true},
		{Message: "chore: lockfile", Files: []string{"package-lock.json"}},
		{Message: "feat: real"},
	}
	cfg := DefaultNoiseConfig()
	cfg.Enabled = false
	filter := NewNoiseFilter(cfg)

	// Act
	filtered := filter.Apply(commits)

	// Assert
	assert.Len(t, filtered, 3)
}

// --- TicketExtractor tests ---

func TestTicketExtractor_JIRAFromBranch(t *testing.T) {
	// Arrange
	extractor := NewTicketExtractor()

	// Act
	tickets := extractor.Extract("feature/AUTH-42-add-login", "add login flow")

	// Assert
	require.Len(t, tickets, 1)
	assert.Equal(t, "AUTH-42", tickets[0])
}

func TestTicketExtractor_GitHubIssueFromMessage(t *testing.T) {
	// Arrange
	extractor := NewTicketExtractor()

	// Act
	tickets := extractor.Extract("main", "fix: resolve crash on startup (#15)")

	// Assert
	require.Len(t, tickets, 1)
	assert.Equal(t, "#15", tickets[0])
}

func TestTicketExtractor_KLRefFromMessage(t *testing.T) {
	// Arrange
	extractor := NewTicketExtractor()

	// Act
	tickets := extractor.Extract("main", "fix: address KL-7 feedback on backlog view")

	// Assert
	require.Len(t, tickets, 1)
	assert.Equal(t, "KL-7", tickets[0])
}

func TestTicketExtractor_MultipleTickets(t *testing.T) {
	// Arrange
	extractor := NewTicketExtractor()

	// Act
	tickets := extractor.Extract("feature/AUTH-42-login", "fix AUTH-42: resolve KL-3 issue (#15)")

	// Assert
	assert.GreaterOrEqual(t, len(tickets), 3)
	assert.Contains(t, tickets, "AUTH-42")
	assert.Contains(t, tickets, "KL-3")
	assert.Contains(t, tickets, "#15")
}

func TestTicketExtractor_NoTickets(t *testing.T) {
	// Arrange
	extractor := NewTicketExtractor()

	// Act
	tickets := extractor.Extract("main", "update readme")

	// Assert
	assert.Empty(t, tickets)
}

// --- TaskGrouper tests ---

func TestTaskGrouper_ConsecutiveSameBranch(t *testing.T) {
	// Arrange
	now := time.Now()
	commits := []Commit{
		{Message: "feat: step 1", Branch: "feature/login", Timestamp: now.Add(-2 * time.Hour)},
		{Message: "feat: step 2", Branch: "feature/login", Timestamp: now.Add(-1 * time.Hour)},
		{Message: "feat: step 3", Branch: "feature/login", Timestamp: now},
	}
	grouper := NewTaskGrouper()

	// Act
	tasks := grouper.Group(commits)

	// Assert
	require.Len(t, tasks, 1)
	assert.Len(t, tasks[0].Commits, 3)
	assert.NotEmpty(t, tasks[0].Summary)
}

func TestTaskGrouper_SingleCommitSingleTask(t *testing.T) {
	// Arrange
	commits := []Commit{
		{Message: "hotfix: urgent patch", Branch: "main", Timestamp: time.Now()},
	}
	grouper := NewTaskGrouper()

	// Act
	tasks := grouper.Group(commits)

	// Assert
	require.Len(t, tasks, 1)
	assert.Len(t, tasks[0].Commits, 1)
}

func TestTaskGrouper_InterleavedBranches(t *testing.T) {
	// Arrange
	now := time.Now()
	commits := []Commit{
		{Message: "feat: login part 1", Branch: "feature/login", Timestamp: now.Add(-3 * time.Hour)},
		{Message: "fix: bug in signup", Branch: "fix/signup-bug", Timestamp: now.Add(-2 * time.Hour)},
		{Message: "feat: login part 2", Branch: "feature/login", Timestamp: now.Add(-1 * time.Hour)},
	}
	grouper := NewTaskGrouper()

	// Act
	tasks := grouper.Group(commits)

	// Assert
	assert.Len(t, tasks, 3)
}

func TestTaskGrouper_EmptyInput(t *testing.T) {
	// Arrange
	grouper := NewTaskGrouper()

	// Act
	tasks := grouper.Group(nil)

	// Assert
	assert.Empty(t, tasks)
}

// --- EffortEstimator tests ---

func TestEffortEstimator_GapUnder2h(t *testing.T) {
	// Arrange
	now := time.Now()
	commits := []Commit{
		{Timestamp: now.Add(-90 * time.Minute)},
		{Timestamp: now},
	}
	estimator := NewEffortEstimator()

	// Act
	effort := estimator.Estimate(commits)

	// Assert
	assert.InDelta(t, 90, effort.Minutes(), 1)
}

func TestEffortEstimator_GapOver2hCapped(t *testing.T) {
	// Arrange
	now := time.Now()
	commits := []Commit{
		{Timestamp: now.Add(-5 * time.Hour)},
		{Timestamp: now},
	}
	estimator := NewEffortEstimator()

	// Act
	effort := estimator.Estimate(commits)

	// Assert — gap exceeds 2h threshold, so should be capped, not 5h
	assert.Less(t, effort.Minutes(), float64(300))
}

func TestEffortEstimator_SingleCommitMinimum(t *testing.T) {
	// Arrange
	commits := []Commit{
		{Timestamp: time.Now()},
	}
	estimator := NewEffortEstimator()

	// Act
	effort := estimator.Estimate(commits)

	// Assert
	assert.Equal(t, 15*time.Minute, effort)
}

func TestEffortEstimator_EmptyInput(t *testing.T) {
	// Arrange
	estimator := NewEffortEstimator()

	// Act
	effort := estimator.Estimate(nil)

	// Assert
	assert.Equal(t, time.Duration(0), effort)
}
