package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/spf13/cobra"
)

func NewBacklogCmd(getStore func() kleio.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backlog",
		Short: "View and manage the synthesized backlog",
	}

	cmd.AddCommand(newBacklogListCmd(getStore))
	cmd.AddCommand(newBacklogShowCmd(getStore))
	cmd.AddCommand(newBacklogPrioritizeCmd(getStore))

	return cmd
}

func newBacklogListCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		status     string
		urgency    string
		importance string
		category   string
		repo       string
		search     string
		assignee   string
		limit      int
		asJSON     bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List synthesized backlog items",
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := getStore().ListBacklogItems(context.Background(), kleio.BacklogFilter{
				Status: status, Urgency: urgency, Importance: importance,
				Category: category, RepoName: repo,
				Search: search, Assignee: assignee, Limit: limit,
			})
			if err != nil {
				return fmt.Errorf("failed to list backlog items: %w", err)
			}

			if asJSON {
				data, _ := json.MarshalIndent(items, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(items) == 0 {
				fmt.Println("No backlog items found.")
				return nil
			}

			for _, item := range items {
				statusIcon := statusSymbol(item.Status)
				label := axisLabel(item.Urgency, item.Importance)
				ticket := ""
				if item.ShortID > 0 {
					ticket = fmt.Sprintf("KL-%d ", item.ShortID)
				}
				fmt.Printf("%s %s%s[%s] %s\n", statusIcon, ticket, shortID(item.ID), label, item.Title)
			}

			fmt.Printf("\n%d items\n", len(items))
			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status (open, in_progress, done, ignored)")
	cmd.Flags().StringVar(&urgency, "urgency", "", "Filter by urgency (low, medium, high)")
	cmd.Flags().StringVar(&importance, "importance", "", "Filter by importance (low, medium, high)")
	cmd.Flags().StringVar(&category, "category", "", "Filter by category")
	cmd.Flags().StringVar(&repo, "repo", "", "Filter by repo")
	cmd.Flags().StringVar(&search, "search", "", "Substring filter on title and summary")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Filter by assignee UUID")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max items to show (0 = no cap)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return cmd
}

func newBacklogShowCmd(getStore func() kleio.Store) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "show [id]",
		Short: "Show a backlog item with details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			item, err := getStore().GetBacklogItem(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("failed to get backlog item: %w", err)
			}

			if asJSON {
				data, _ := json.MarshalIndent(item, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("ID:         %s\n", item.ID)
			fmt.Printf("Title:      %s\n", item.Title)
			fmt.Printf("Summary:    %s\n", item.Summary)
			fmt.Printf("Category:   %s\n", item.Category)
			fmt.Printf("Urgency:    %s\n", item.Urgency)
			fmt.Printf("Importance: %s\n", item.Importance)
			fmt.Printf("Status:     %s\n", item.Status)
			if item.RepoName != "" {
				fmt.Printf("Repo:       %s\n", item.RepoName)
			}
			fmt.Printf("Created:    %s\n", item.CreatedAt)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func newBacklogPrioritizeCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		urgency    string
		importance string
		status     string
	)

	validAxes := map[string]bool{"low": true, "medium": true, "high": true}
	validStatuses := map[string]bool{"open": true, "in_progress": true, "done": true, "ignored": true}

	cmd := &cobra.Command{
		Use:   "prioritize [id]",
		Short: "Set urgency, importance, and/or status of a backlog item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			update := &kleio.BacklogItem{}
			hasUpdate := false

			if urgency != "" {
				if !validAxes[urgency] {
					return fmt.Errorf("invalid urgency '%s', valid values: low, medium, high", urgency)
				}
				update.Urgency = urgency
				hasUpdate = true
			}
			if importance != "" {
				if !validAxes[importance] {
					return fmt.Errorf("invalid importance '%s', valid values: low, medium, high", importance)
				}
				update.Importance = importance
				hasUpdate = true
			}
			if status != "" {
				if !validStatuses[status] {
					return fmt.Errorf("invalid status '%s', valid values: open, in_progress, done, ignored", status)
				}
				update.Status = status
				hasUpdate = true
			}
			if !hasUpdate {
				return fmt.Errorf("specify --urgency, --importance, and/or --status")
			}

			store := getStore()
			if err := store.UpdateBacklogItem(context.Background(), args[0], update); err != nil {
				return fmt.Errorf("failed to update: %w", err)
			}

			got, err := store.GetBacklogItem(context.Background(), args[0])
			if err != nil {
				fmt.Printf("Updated %s\n", args[0])
				return nil
			}
			fmt.Printf("Updated %s: urgency=%s importance=%s status=%s\n",
				shortID(got.ID), got.Urgency, got.Importance, got.Status)
			return nil
		},
	}

	cmd.Flags().StringVar(&urgency, "urgency", "", "Set urgency (low, medium, high)")
	cmd.Flags().StringVar(&importance, "importance", "", "Set importance (low, medium, high)")
	cmd.Flags().StringVar(&status, "status", "", "Set status (open, in_progress, done, ignored)")

	return cmd
}

func statusSymbol(status string) string {
	switch status {
	case "done":
		return "[x]"
	case "in_progress":
		return "[>]"
	case "ignored":
		return "[-]"
	default:
		return "[ ]"
	}
}

func axisLabel(urgency, importance string) string {
	return fmt.Sprintf("U:%s I:%s", shortLevel(urgency), shortLevel(importance))
}

func shortLevel(level string) string {
	switch level {
	case "high":
		return "H"
	case "medium":
		return "M"
	default:
		return "L"
	}
}

// shortID and oneLine are shared helpers used by multiple commands.
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
