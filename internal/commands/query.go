package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/spf13/cobra"
)

// NewQueryCmd builds the `kleio query ...` subcommand group, mirroring Ask's
// retrieval surface (search_captures / semantic_search / get_capture_detail /
// search_backlog / get_backlog_detail) so humans and agent CLIs can hit the
// same memory read API the LLM uses inside the Ask agentic loop.
func NewQueryCmd(getClient func() *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query workspace memory (captures + backlog) using the same surface Ask exposes to the LLM",
		Long: `Query workspace memory: filter captures by time/signal/repo/keyword,
embedding-search them by meaning, fetch full detail for one capture, search
backlog items, or fetch full detail for one backlog item (UUID or KL-N).

These commands hit the same MemoryReadService surface the Ask agentic loop
calls via tool-use, so output is consistent across web Ask, MCP read tools,
and the CLI.`,
	}

	cmd.AddCommand(newQueryCapturesCmd(getClient))
	cmd.AddCommand(newQuerySemanticCmd(getClient))
	cmd.AddCommand(newQueryCaptureCmd(getClient))
	cmd.AddCommand(newQueryBacklogCmd(getClient))
	cmd.AddCommand(newQueryBacklogShowCmd(getClient))

	return cmd
}

func newQueryCapturesCmd(getClient func() *client.Client) *cobra.Command {
	var (
		since      string
		until      string
		signalType string
		repoName   string
		keyword    string
		limit      int
		asJSON     bool
	)

	cmd := &cobra.Command{
		Use:   "captures",
		Short: "List captures filtered by time/signal/repo/keyword",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := getClient().SearchCaptures(client.SearchCapturesQuery{
				Since:      since,
				Until:      until,
				SignalType: signalType,
				RepoName:   repoName,
				Keyword:    keyword,
				Limit:      limit,
			})
			if err != nil {
				return fmt.Errorf("search captures: %w", err)
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), out)
			}
			if len(out.Captures) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No captures found.")
				return nil
			}
			for _, h := range out.Captures {
				printCaptureRow(cmd.OutOrStdout(), h)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d captures\n", len(out.Captures))
			return nil
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "ISO-8601 datetime/date for start of window (inclusive)")
	cmd.Flags().StringVar(&until, "until", "", "ISO-8601 datetime/date for end of window (inclusive)")
	cmd.Flags().StringVar(&signalType, "signal-type", "", "Filter by signal type (decision, checkpoint, work_item, git_commit, ...)")
	cmd.Flags().StringVar(&repoName, "repo", "", "Substring match on repo_name (case-insensitive)")
	cmd.Flags().StringVar(&keyword, "keyword", "", "Case-insensitive substring search in capture content")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max captures to return (default 10, max 25)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return cmd
}

func newQuerySemanticCmd(getClient func() *client.Client) *cobra.Command {
	var (
		limit  int
		asJSON bool
	)

	cmd := &cobra.Command{
		Use:   "semantic [query]",
		Short: "Embedding similarity search over captures (thematic/conceptual)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := strings.TrimSpace(strings.Join(args, " "))
			if q == "" {
				return fmt.Errorf("query is required")
			}
			out, err := getClient().SemanticSearchCaptures(client.SemanticSearchQuery{Query: q, Limit: limit})
			if err != nil {
				return fmt.Errorf("semantic search: %w", err)
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), out)
			}
			if out.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Note: %s\n", out.Note)
			}
			if len(out.Captures) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No captures found.")
				return nil
			}
			for _, h := range out.Captures {
				fmt.Fprintf(cmd.OutOrStdout(), "  (%.2f) ", h.Similarity)
				printCaptureRow(cmd.OutOrStdout(), h)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d captures\n", len(out.Captures))
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 0, "Max captures to return (default 10, max 15)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return cmd
}

func newQueryCaptureCmd(getClient func() *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capture [id]",
		Short: "Show one capture by UUID with checkpoint/decision/topics/targets",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := getClient().GetCaptureDetail(args[0])
			if err != nil {
				return fmt.Errorf("get capture detail: %w", err)
			}
			var pretty map[string]any
			if err := json.Unmarshal(raw, &pretty); err != nil {
				_, _ = cmd.OutOrStdout().Write(raw)
				return nil
			}
			b, _ := json.MarshalIndent(pretty, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		},
	}
	return cmd
}

func newQueryBacklogCmd(getClient func() *client.Client) *cobra.Command {
	var (
		status     string
		category   string
		urgency    string
		importance string
		keyword    string
		ticketID   string
		repoName   string
		limit      int
		asJSON     bool
	)

	cmd := &cobra.Command{
		Use:   "backlog",
		Short: "Search backlog items via the memory read surface",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := getClient().SearchBacklogMemory(client.SearchBacklogQuery{
				Status:     status,
				Category:   category,
				Urgency:    urgency,
				Importance: importance,
				Keyword:    keyword,
				TicketID:   ticketID,
				RepoName:   repoName,
				Limit:      limit,
			})
			if err != nil {
				return fmt.Errorf("search backlog: %w", err)
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), out)
			}
			if len(out.BacklogItems) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No backlog items found.")
				return nil
			}
			for _, h := range out.BacklogItems {
				ticket := ""
				if h.ShortID != nil {
					ticket = fmt.Sprintf("KL-%d ", *h.ShortID)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %s%s [%s] %s — %s\n", ticket, shortID(h.ID), h.Status, h.Title, oneLine(h.Summary, 80))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d items\n", out.TotalReturned)
			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status (new, ready, in_progress, reviewed, done, ignored, implemented, blocked)")
	cmd.Flags().StringVar(&category, "category", "", "Filter by category (bug, feature, tech_debt, refactor, docs, research, other)")
	cmd.Flags().StringVar(&urgency, "urgency", "", "Filter by urgency (low, medium, high)")
	cmd.Flags().StringVar(&importance, "importance", "", "Filter by importance (low, medium, high)")
	cmd.Flags().StringVar(&keyword, "keyword", "", "Case-insensitive substring search in title/summary")
	cmd.Flags().StringVar(&ticketID, "ticket", "", "Match a single backlog item by short id (KL-42 or 42)")
	cmd.Flags().StringVar(&repoName, "repo", "", "Substring match on repo_name (case-insensitive)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max items to return (default 20, max 50)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return cmd
}

func newQueryBacklogShowCmd(getClient func() *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backlog-show [id]",
		Short: "Show one backlog item (UUID or KL-N) with linked captures",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := getClient().GetBacklogDetail(args[0])
			if err != nil {
				return fmt.Errorf("get backlog detail: %w", err)
			}
			var pretty map[string]any
			if err := json.Unmarshal(raw, &pretty); err != nil {
				_, _ = cmd.OutOrStdout().Write(raw)
				return nil
			}
			b, _ := json.MarshalIndent(pretty, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		},
	}
	return cmd
}

func printCaptureRow(w io.Writer, h client.MemoryCaptureHit) {
	repo := ""
	if h.RepoName != "" {
		repo = " [" + h.RepoName + "]"
	}
	fmt.Fprintf(w, "  %s [%s]%s %s\n", shortID(h.CaptureID), h.SignalType, repo, oneLine(h.Content, 100))
}

func printJSON(w io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(b))
	return nil
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func oneLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
