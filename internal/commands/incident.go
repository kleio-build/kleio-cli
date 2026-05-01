package commands

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/ai"
	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/kleio-build/kleio-cli/internal/render"
	"github.com/spf13/cobra"
)

// SuspectChange is a commit or event flagged as potentially related to the bug.
type SuspectChange struct {
	Kind      string  `json:"kind"`
	SHA       string  `json:"sha,omitempty"`
	EventID   string  `json:"event_id,omitempty"`
	Summary   string  `json:"summary"`
	Timestamp string  `json:"timestamp"`
	Score     float64 `json:"score"`
	Reason    string  `json:"reason"`
}

// NewIncidentCmd creates the `kleio incident` command.
func NewIncidentCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		files         []string
		since         string
		format        string
		output        string
		verbose       bool
		noLLM         bool
		asJSON        bool
		noInteractive bool
		allRepos      bool
	)

	cmd := &cobra.Command{
		Use:   "incident [signal]",
		Short: "Find what changed that could explain a bug",
		Long: `Parse a bug signal and find suspicious recent changes.

Examples:
  kleio incident "checkout form returns 500"
  kleio incident --files src/payments,src/checkout
  kleio incident --since 3d "login fails"
  kleio incident --format pdf --output incident.pdf "500 errors"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			signal := ""
			if len(args) > 0 {
				signal = args[0]
			}
			store := getStore()

			provider := ai.AutoDetect(ai.LoadConfig())
			if noLLM {
				provider = ai.Noop{}
			}
			eng := engine.New(store, provider).WithExpander(loadAnchorExpander(provider))

			sinceTime := parseSinceOrDefault(since, 3)
			_ = noInteractive

			scopeRepo := ""
			if !allRepos {
				scopeRepo = currentRepoName()
			}

			entries, err := buildIncidentEntries(context.Background(), eng, store, signal, files, sinceTime, scopeRepo)
			if err != nil {
				return fmt.Errorf("incident analysis failed: %w", err)
			}

			if len(entries) == 0 {
				if !noInteractive && isInteractive() {
					result := runIncidentRefinement(os.Stdin, os.Stderr, eng, store, signal, files, sinceTime)
					if result != nil {
						entries = incidentReportToEntries(result)
					}
				}
				if len(entries) == 0 {
					fmt.Fprintln(os.Stderr, "No suspicious changes found in the given time window.")
					os.Exit(1)
				}
			}

			if asJSON && format == "" {
				format = "json"
			}

			anchor := signal
			if anchor == "" {
				anchor = "incident"
			}

			report := eng.BuildReport(context.Background(), anchor, "incident", entries)
			if !noLLM {
				_ = report.Enrich(context.Background(), provider)
			}
			return render.Render(os.Stdout, format, output, report, verbose)
		},
	}

	cmd.Flags().StringSliceVar(&files, "files", nil, "File paths to investigate")
	cmd.Flags().StringVar(&since, "since", "3d", "Time window (default 3 days)")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text, md, html, pdf, json")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write output to file")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Include raw timeline in output")
	cmd.Flags().BoolVar(&noLLM, "no-llm", false, "Skip LLM enrichment even if available")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON (shorthand for --format json)")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "Suppress prompts")
	cmd.Flags().BoolVar(&allRepos, "all-repos", false, "Include commits and events from all indexed repositories (default: current repo only)")

	return cmd
}

func buildIncidentEntries(
	ctx context.Context,
	eng *engine.Engine,
	store kleio.Store,
	signal string,
	filePaths []string,
	sinceTime time.Time,
	repoName string,
) ([]engine.TimelineEntry, error) {
	stats := engine.ComputeCorpusStats(ctx, store, repoName)
	params := engine.DeriveParams(stats)
	keywords := extractKeywords(signal)

	commits, err := store.QueryCommits(ctx, kleio.CommitFilter{
		Since:    sinceTime.Format(time.RFC3339),
		RepoName: repoName,
		Limit:    200,
	})
	if err != nil {
		return nil, err
	}

	type scoredCommit struct {
		commit kleio.Commit
		score  float64
	}
	var scored []scoredCommit
	for _, c := range commits {
		score := scoreCommitForIncidentAdaptive(c, keywords, filePaths, sinceTime, params)
		scored = append(scored, scoredCommit{commit: c, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	topK := params.TopK
	if topK > len(scored) {
		topK = len(scored)
	}

	var entries []engine.TimelineEntry
	lowRelevance := true
	for i := 0; i < topK; i++ {
		sc := scored[i]
		if sc.score < params.ScoreFloor {
			break
		}
		if sc.score >= 0.2 {
			lowRelevance = false
		}
		t, _ := time.Parse(time.RFC3339, sc.commit.CommittedAt)
		if t.IsZero() {
			t, _ = time.Parse("2006-01-02T15:04:05Z07:00", sc.commit.CommittedAt)
		}
		entries = append(entries, engine.TimelineEntry{
			Timestamp: t,
			Kind:      kleio.SignalTypeGitCommit,
			Summary:   firstLine(sc.commit.Message),
			SHA:       sc.commit.SHA,
		})
	}

	_ = lowRelevance

	results, err := eng.Search(ctx, signal, params.TopK)
	if err == nil {
		for _, r := range results {
			if r.Kind == "event" {
				t, _ := time.Parse(time.RFC3339, r.CreatedAt)
				entries = append(entries, engine.TimelineEntry{
					Timestamp: t,
					Kind:      kleio.SignalTypeWorkItem,
					Summary:   firstLine(r.Content),
					EventID:   r.ID,
				})
			}
		}
	}

	return entries, nil
}

// IncidentReport is kept for backward compat with the interactive refinement loop.
type IncidentReport struct {
	Signal     string          `json:"signal"`
	TimeWindow string          `json:"time_window"`
	Suspects   []SuspectChange `json:"suspects"`
	Summary    string          `json:"summary"`
}

func incidentReportToEntries(ir *IncidentReport) []engine.TimelineEntry {
	var entries []engine.TimelineEntry
	for _, s := range ir.Suspects {
		t, _ := time.Parse(time.RFC3339, s.Timestamp)
		entries = append(entries, engine.TimelineEntry{
			Timestamp: t,
			Kind:      s.Kind,
			Summary:   s.Summary,
			SHA:       s.SHA,
			EventID:   s.EventID,
		})
	}
	return entries
}

func scoreCommitForIncident(c kleio.Commit, keywords, filePaths []string, sinceTime time.Time) float64 {
	return scoreCommitForIncidentAdaptive(c, keywords, filePaths, sinceTime, engine.DefaultParams())
}

func scoreCommitForIncidentAdaptive(c kleio.Commit, keywords, filePaths []string, sinceTime time.Time, params engine.AdaptiveParams) float64 {
	score := 0.0
	score += 0.35 * engine.RecencyScoreWithHalfLife(c.CommittedAt, params.RecencyHalfLife)

	msg := strings.ToLower(c.Message)
	for _, kw := range keywords {
		if strings.Contains(msg, strings.ToLower(kw)) {
			score += 0.15
		}
	}
	if c.FilesChanged > 50 {
		score += 0.10
	}
	for _, fp := range filePaths {
		if strings.Contains(msg, fp) {
			score += 0.20
		}
	}
	errorKeywords := []string{"fix", "bug", "error", "crash", "panic", "revert", "hotfix"}
	for _, ek := range errorKeywords {
		if strings.Contains(msg, ek) {
			score += 0.10
			break
		}
	}
	if score > 1.0 {
		score = 1.0
	}
	return score
}

func classifyRisk(c kleio.Commit, keywords, filePaths []string) string {
	msg := strings.ToLower(c.Message)
	if strings.Contains(msg, "revert") {
		return "revert"
	}
	if c.FilesChanged > 50 {
		return "large_change"
	}
	for _, ek := range []string{"fix", "bug", "hotfix"} {
		if strings.Contains(msg, ek) {
			return "bug_fix_nearby"
		}
	}
	for _, kw := range keywords {
		if strings.Contains(msg, strings.ToLower(kw)) {
			return "keyword_match"
		}
	}
	return "temporal_proximity"
}

func extractKeywords(signal string) []string {
	if signal == "" {
		return nil
	}
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "been": true, "be": true, "have": true,
		"has": true, "had": true, "do": true, "does": true, "did": true,
		"will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "shall": true, "can": true,
		"this": true, "that": true, "these": true, "those": true,
		"i": true, "you": true, "he": true, "she": true, "it": true,
		"we": true, "they": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
		"of": true, "with": true, "by": true, "from": true, "not": true,
	}

	words := strings.Fields(strings.ToLower(signal))
	var keywords []string
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:'\"")
		if len(w) > 2 && !stopWords[w] {
			keywords = append(keywords, w)
		}
	}
	return keywords
}

func sortSuspects(suspects []SuspectChange) {
	for i := 0; i < len(suspects); i++ {
		for j := i + 1; j < len(suspects); j++ {
			if suspects[j].Score > suspects[i].Score {
				suspects[i], suspects[j] = suspects[j], suspects[i]
			}
		}
	}
}
