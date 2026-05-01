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
	"github.com/kleio-build/kleio-cli/internal/entity"
	"github.com/kleio-build/kleio-cli/internal/gitreader"
	"github.com/kleio-build/kleio-cli/internal/render"
	"github.com/spf13/cobra"
)

// NewExplainCmd creates the `kleio explain` command.
func NewExplainCmd(getStore func() kleio.Store) *cobra.Command {
	var (
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
		Use:   "explain <source> <target>",
		Short: "Explain what changed between two points and why",
		Long: `Compare two git refs, branches, or tags and explain changes.

Examples:
  kleio explain HEAD~5 HEAD
  kleio explain main feature/auth-refactor
  kleio explain v1.2.0 v1.3.0
  kleio explain --format md HEAD~10 HEAD`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			source, target := args[0], args[1]
			store := getStore()

			provider := ai.AutoDetect(ai.LoadConfig())
			if noLLM {
				provider = ai.Noop{}
			}
			eng := engine.New(store, provider).WithExpander(loadAnchorExpander(provider))
			_ = noInteractive

			anchor := fmt.Sprintf("%s..%s", source, target)
			scopeRepo := ""
			if !allRepos {
				scopeRepo = currentRepoName()
			}
			entries := buildExplainEntries(store, eng, source, target, since, scopeRepo)

			if asJSON && format == "" {
				format = "json"
			}

			stats := engine.ComputeCorpusStats(context.Background(), store, scopeRepo)
			params := engine.DeriveParams(stats)
			report := eng.BuildReportWithOptions(context.Background(), anchor, "explain", entries, engine.ReportOptions{Stats: &stats})
			if !noLLM {
				_ = report.Enrich(context.Background(), provider)
			}
			renderOpts := render.Options{
				Verbose:   verbose,
				RenderCap: params.RenderCap,
				DeferCap:  params.RenderDeferCap,
			}
			return render.RenderWithOptions(os.Stdout, format, output, report, renderOpts)
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "Time window filter")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text, md, html, pdf, json")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write output to file")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Include raw timeline in output")
	cmd.Flags().BoolVar(&noLLM, "no-llm", false, "Skip LLM enrichment even if available")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON (shorthand for --format json)")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "Suppress prompts")
	cmd.Flags().BoolVar(&allRepos, "all-repos", false, "Include events from all indexed repositories (default: current repo only)")

	return cmd
}

func buildExplainEntries(store kleio.Store, eng *engine.Engine, source, target, since, repoName string) []engine.TimelineEntry {
	ctx := context.Background()

	rangeCommits, err := gitreader.CommitRange(".", source, target)
	if err != nil {
		sinceTime, _ := parseSince(since)
		entries, tlErr := eng.TimelineScoped(ctx, "", repoName, sinceTime)
		if tlErr != nil {
			return nil
		}
		return entries
	}

	stats := engine.ComputeCorpusStats(ctx, store, repoName)
	params := engine.DeriveParams(stats)

	var entries []engine.TimelineEntry
	var earliest, latest time.Time
	var allFiles []string
	var keywords []string

	commitEntitySet := map[string]bool{}
	for _, rc := range rangeCommits {
		entries = append(entries, engine.TimelineEntry{
			Timestamp: rc.Timestamp,
			Kind:      kleio.SignalTypeGitCommit,
			Summary:   firstLine(rc.Message),
			SHA:       rc.Hash,
			FilePaths: rc.Files,
		})
		if earliest.IsZero() || rc.Timestamp.Before(earliest) {
			earliest = rc.Timestamp
		}
		if latest.IsZero() || rc.Timestamp.After(latest) {
			latest = rc.Timestamp
		}
		allFiles = append(allFiles, rc.Files...)
		for _, word := range strings.Fields(firstLine(rc.Message)) {
			if len(word) > 3 {
				keywords = appendUnique(keywords, strings.ToLower(word))
			}
		}
		// Extract entities from commits for entity-overlap scoring.
		for _, ext := range entity.Extract(rc.Message, kleio.AliasSourceCommitMessage) {
			commitEntitySet[ext.Kind+":"+entity.NormalizeLabel(ext.Kind, ext.Value)] = true
		}
		for _, f := range rc.Files {
			commitEntitySet[kleio.EntityKindFile+":"+entity.NormalizeLabel(kleio.EntityKindFile, f)] = true
		}
	}

	filter := kleio.EventFilter{
		RepoName: repoName,
		Limit:    params.TopK * 2,
	}
	if !earliest.IsZero() {
		filter.CreatedAfter = earliest.Add(-24 * time.Hour).Format(time.RFC3339)
	}
	if !latest.IsZero() {
		filter.CreatedBefore = latest.Add(24 * time.Hour).Format(time.RFC3339)
	}

	events, _ := store.ListEvents(ctx, filter)

	keywordQuery := strings.Join(keywords, " ")
	type scoredEvent struct {
		entry engine.TimelineEntry
		score float64
	}
	var scored []scoredEvent
	seenIDs := make(map[string]bool)

	for _, ev := range events {
		t, _ := time.Parse(time.RFC3339, ev.CreatedAt)
		entityScore := entityOverlapScore(ev.Content, commitEntitySet)
		score := engine.RecencyScoreWithHalfLife(ev.CreatedAt, params.RecencyHalfLife)*0.20 +
			keywordScore(ev.Content, keywordQuery)*0.30 +
			fileOverlapScore(ev.FilePath, allFiles)*0.20 +
			idRefScore(ev.Content, rangeCommits)*0.15 +
			entityScore*0.15

		if score < params.ScoreFloor {
			continue
		}

		seenIDs[ev.ID] = true
		scored = append(scored, scoredEvent{
			entry: engine.TimelineEntry{
				Timestamp: t,
				Kind:      ev.SignalType,
				Summary:   firstLine(ev.Content),
				EventID:   ev.ID,
			},
			score: score,
		})
	}

	if len(keywords) > 0 {
		searchResults, err := eng.Search(ctx, keywordQuery, params.TopK)
		if err == nil {
			for _, sr := range searchResults {
				if sr.Kind != "event" || seenIDs[sr.ID] {
					continue
				}
				seenIDs[sr.ID] = true
				t, _ := time.Parse(time.RFC3339, sr.CreatedAt)
				scored = append(scored, scoredEvent{
					entry: engine.TimelineEntry{
						Timestamp: t,
						Kind:      sr.SignalType,
						Summary:   firstLine(sr.Content),
						EventID:   sr.ID,
					},
					score: sr.EngineScore,
				})
			}
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	topK := params.TopK
	if topK > len(scored) {
		topK = len(scored)
	}
	for i := 0; i < topK; i++ {
		entries = append(entries, scored[i].entry)
	}

	return entries
}

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

func fileOverlapScore(evPath string, commitFiles []string) float64 {
	if evPath == "" || len(commitFiles) == 0 {
		return 0
	}
	lower := strings.ToLower(evPath)
	for _, f := range commitFiles {
		if strings.Contains(lower, strings.ToLower(f)) || strings.Contains(strings.ToLower(f), lower) {
			return 1.0
		}
	}
	return 0
}

func entityOverlapScore(content string, commitEntitySet map[string]bool) float64 {
	if len(commitEntitySet) == 0 {
		return 0
	}
	extracted := entity.Extract(content, "")
	if len(extracted) == 0 {
		return 0
	}
	hits := 0
	for _, ext := range extracted {
		key := ext.Kind + ":" + entity.NormalizeLabel(ext.Kind, ext.Value)
		if commitEntitySet[key] {
			hits++
		}
	}
	if hits == 0 {
		return 0
	}
	return min(1.0, float64(hits)/float64(len(extracted)))
}

func idRefScore(content string, commits []gitreader.Commit) float64 {
	for _, c := range commits {
		if len(c.Hash) >= 7 && strings.Contains(content, c.Hash[:7]) {
			return 1.0
		}
	}
	return 0
}

func extractSubsystem(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 1 {
		return "root"
	}
	return parts[0]
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

func parseSinceOrDefault(s string, defaultDays int) time.Time {
	if s != "" {
		if t, err := parseSince(s); err == nil && !t.IsZero() {
			return t
		}
	}
	return time.Now().Add(-time.Duration(defaultDays) * 24 * time.Hour)
}
