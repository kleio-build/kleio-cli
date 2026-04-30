package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/spf13/cobra"
)

func NewQueryCmd(getStore func() kleio.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query Kleio memory (events + backlog)",
		Long: `Query local or cloud memory: filter events by time/signal/repo/keyword,
search by text, fetch full detail for one event, or search backlog items.`,
	}

	cmd.AddCommand(newQueryCapturesCmd(getStore))
	cmd.AddCommand(newQuerySearchCmd(getStore))
	cmd.AddCommand(newQueryCaptureCmd(getStore))
	cmd.AddCommand(newQueryBacklogCmd(getStore))
	cmd.AddCommand(newQueryBacklogShowCmd(getStore))

	return cmd
}

func newQueryCapturesCmd(getStore func() kleio.Store) *cobra.Command {
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
		Short: "List events filtered by time/signal/repo/keyword",
		RunE: func(cmd *cobra.Command, args []string) error {
			filter := kleio.EventFilter{
				SignalType:    signalType,
				RepoName:     repoName,
				CreatedAfter:  since,
				CreatedBefore: until,
				Limit:         limit,
			}

			store := getStore()

			if keyword != "" {
				results, err := store.Search(context.Background(), keyword, kleio.SearchOpts{
					RepoName:   repoName,
					SignalType: signalType,
					Since:      since,
					Limit:      limit,
				})
				if err != nil {
					return fmt.Errorf("search: %w", err)
				}
				if asJSON {
					return printJSON(cmd.OutOrStdout(), results)
				}
				if len(results) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No results found.")
					return nil
				}
				for _, r := range results {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s [%s] %s\n",
						shortID(r.ID), r.Kind, oneLine(r.Content, 100))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d results\n", len(results))
				return nil
			}

			events, err := store.ListEvents(context.Background(), filter)
			if err != nil {
				return fmt.Errorf("list events: %w", err)
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), events)
			}
			if len(events) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No events found.")
				return nil
			}
			for _, e := range events {
				repo := ""
				if e.RepoName != "" {
					repo = " [" + e.RepoName + "]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %s [%s]%s %s\n",
					shortID(e.ID), e.SignalType, repo, oneLine(e.Content, 100))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d events\n", len(events))
			return nil
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "Start of time window (RFC3339 or date)")
	cmd.Flags().StringVar(&until, "until", "", "End of time window (RFC3339 or date)")
	cmd.Flags().StringVar(&signalType, "signal-type", "", "Filter by signal type")
	cmd.Flags().StringVar(&repoName, "repo", "", "Filter by repo name")
	cmd.Flags().StringVar(&keyword, "keyword", "", "Search in content")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max results")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return cmd
}

func newQuerySearchCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		limit  int
		asJSON bool
	)

	cmd := &cobra.Command{
		Use:   "semantic [query]",
		Short: "Search events by text (local) or embeddings (cloud)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := strings.TrimSpace(strings.Join(args, " "))
			if q == "" {
				return fmt.Errorf("query is required")
			}
			results, err := getStore().Search(context.Background(), q, kleio.SearchOpts{Limit: limit})
			if err != nil {
				return fmt.Errorf("search: %w", err)
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), results)
			}
			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No results found.")
				return nil
			}
			for _, r := range results {
				fmt.Fprintf(cmd.OutOrStdout(), "  (%.2f) %s [%s] %s\n",
					r.Score, shortID(r.ID), r.Kind, oneLine(r.Content, 100))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d results\n", len(results))
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 0, "Max results")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return cmd
}

func newQueryCaptureCmd(getStore func() kleio.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capture [id]",
		Short: "Show one event by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			evt, err := getStore().GetEvent(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("get event: %w", err)
			}
			b, _ := json.MarshalIndent(evt, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		},
	}
	return cmd
}

func newQueryBacklogCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		status     string
		category   string
		urgency    string
		importance string
		search     string
		repoName   string
		limit      int
		asJSON     bool
	)

	cmd := &cobra.Command{
		Use:   "backlog",
		Short: "Search backlog items",
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := getStore().ListBacklogItems(context.Background(), kleio.BacklogFilter{
				Status:     status,
				Category:   category,
				Urgency:    urgency,
				Importance: importance,
				Search:     search,
				RepoName:   repoName,
				Limit:      limit,
			})
			if err != nil {
				return fmt.Errorf("search backlog: %w", err)
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), items)
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No backlog items found.")
				return nil
			}
			for _, h := range items {
				ticket := ""
				if h.ShortID > 0 {
					ticket = fmt.Sprintf("KL-%d ", h.ShortID)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %s%s [%s] %s — %s\n",
					ticket, shortID(h.ID), h.Status, h.Title, oneLine(h.Summary, 80))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d items\n", len(items))
			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&category, "category", "", "Filter by category")
	cmd.Flags().StringVar(&urgency, "urgency", "", "Filter by urgency")
	cmd.Flags().StringVar(&importance, "importance", "", "Filter by importance")
	cmd.Flags().StringVar(&search, "keyword", "", "Search in title/summary")
	cmd.Flags().StringVar(&repoName, "repo", "", "Filter by repo")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max items")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return cmd
}

func newQueryBacklogShowCmd(getStore func() kleio.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backlog-show [id]",
		Short: "Show one backlog item by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			item, err := getStore().GetBacklogItem(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("get backlog item: %w", err)
			}
			b, _ := json.MarshalIndent(item, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		},
	}
	return cmd
}

func printJSON(w io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(b))
	return nil
}
