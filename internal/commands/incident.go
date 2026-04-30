package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/ai"
	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/spf13/cobra"
)

// IncidentReport is the structured output of `kleio incident`.
type IncidentReport struct {
	Signal      string           `json:"signal"`
	TimeWindow  string           `json:"time_window"`
	Suspects    []SuspectChange  `json:"suspects"`
	Summary     string           `json:"summary"`
}

// SuspectChange is a commit or event flagged as potentially related to the bug.
type SuspectChange struct {
	Kind       string  `json:"kind"`
	SHA        string  `json:"sha,omitempty"`
	EventID    string  `json:"event_id,omitempty"`
	Summary    string  `json:"summary"`
	Timestamp  string  `json:"timestamp"`
	Score      float64 `json:"score"`
	Reason     string  `json:"reason"`
}

// NewIncidentCmd creates the `kleio incident` command.
func NewIncidentCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		files         []string
		since         string
		asJSON        bool
		noInteractive bool
	)

	cmd := &cobra.Command{
		Use:   "incident [signal]",
		Short: "Find what changed that could explain a bug",
		Long: `Parse a bug signal and find suspicious recent changes.

Examples:
  kleio incident "checkout form returns 500"
  kleio incident --files src/payments,src/checkout
  kleio incident --since 3d "login fails"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			signal := ""
			if len(args) > 0 {
				signal = args[0]
			}
			store := getStore()

			provider, _ := ai.ResolveProvider(ai.LoadConfig())
			eng := engine.New(store, provider)

			sinceTime := parseSinceOrDefault(since, 3)
			_ = noInteractive

			report, err := buildIncidentReport(context.Background(), eng, store, signal, files, sinceTime)
			if err != nil {
				return fmt.Errorf("incident analysis failed: %w", err)
			}

			if len(report.Suspects) == 0 {
				fmt.Fprintln(os.Stderr, "No suspicious changes found in the given time window.")
				os.Exit(1)
			}

			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(report)
			}

			return renderIncidentReport(os.Stdout, report)
		},
	}

	cmd.Flags().StringSliceVar(&files, "files", nil, "File paths to investigate")
	cmd.Flags().StringVar(&since, "since", "3d", "Time window (default 3 days)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "Suppress prompts")

	return cmd
}

func buildIncidentReport(
	ctx context.Context,
	eng *engine.Engine,
	store kleio.Store,
	signal string,
	filePaths []string,
	sinceTime time.Time,
) (*IncidentReport, error) {
	keywords := extractKeywords(signal)

	var suspects []SuspectChange

	commits, err := store.QueryCommits(ctx, kleio.CommitFilter{
		Since: sinceTime.Format(time.RFC3339),
		Limit: 200,
	})
	if err != nil {
		return nil, err
	}

	for _, c := range commits {
		score := scoreCommitForIncident(c, keywords, filePaths, sinceTime)
		if score < 0.1 {
			continue
		}

		reason := classifyRisk(c, keywords, filePaths)
		suspects = append(suspects, SuspectChange{
			Kind:      "commit",
			SHA:       c.SHA,
			Summary:   firstLine(c.Message),
			Timestamp: c.CommittedAt,
			Score:     score,
			Reason:    reason,
		})
	}

	results, err := eng.Search(ctx, signal, 20)
	if err == nil {
		for _, r := range results {
			if r.Kind == "event" {
				suspects = append(suspects, SuspectChange{
					Kind:      "event",
					EventID:   r.ID,
					Summary:   firstLine(r.Content),
					Timestamp: r.CreatedAt,
					Score:     r.EngineScore * 0.8,
					Reason:    "keyword_match",
				})
			}
		}
	}

	sortSuspects(suspects)
	if len(suspects) > 20 {
		suspects = suspects[:20]
	}

	summary := fmt.Sprintf("Found %d suspicious change(s) since %s",
		len(suspects), sinceTime.Format("2006-01-02"))

	return &IncidentReport{
		Signal:     signal,
		TimeWindow: sinceTime.Format(time.RFC3339),
		Suspects:   suspects,
		Summary:    summary,
	}, nil
}

func scoreCommitForIncident(c kleio.Commit, keywords, filePaths []string, sinceTime time.Time) float64 {
	score := 0.0

	score += 0.35 * engine.ExportRecencyScore(c.CommittedAt)

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

func renderIncidentReport(w *os.File, report *IncidentReport) error {
	fmt.Fprintf(w, "=== Incident Analysis ===\n\n")
	if report.Signal != "" {
		fmt.Fprintf(w, "Signal: %q\n", report.Signal)
	}
	fmt.Fprintf(w, "Window: since %s\n", report.TimeWindow)
	fmt.Fprintf(w, "Suspects: %d\n\n", len(report.Suspects))

	for i, s := range report.Suspects {
		icon := kindIcon(s.Kind)
		ts, _ := time.Parse(time.RFC3339, s.Timestamp)
		fmt.Fprintf(w, "  %d. %s %.0f%% [%s] %s %s\n",
			i+1, icon, s.Score*100, s.Reason, ts.Format("2006-01-02 15:04"), s.Summary)
		if s.SHA != "" {
			fmt.Fprintf(w, "     sha: %s\n", s.SHA)
		}
	}

	fmt.Fprintf(w, "\n%s\n", report.Summary)
	return nil
}
