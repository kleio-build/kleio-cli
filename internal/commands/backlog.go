package commands

import (
	"encoding/json"
	"fmt"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/spf13/cobra"
)

func NewBacklogCmd(getClient func() *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backlog",
		Short: "View and manage the synthesized backlog",
	}

	cmd.AddCommand(newBacklogListCmd(getClient))
	cmd.AddCommand(newBacklogShowCmd(getClient))
	cmd.AddCommand(newBacklogPrioritizeCmd(getClient))

	return cmd
}

func newBacklogListCmd(getClient func() *client.Client) *cobra.Command {
	var (
		status   string
		priority string
		category string
		repo     string
		asJSON   bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List synthesized backlog items",
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := getClient().ListBacklogItems(status, priority, category, repo)
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
				priorityLabel := priorityLabel(item.Priority)
				fmt.Printf("%s %s [%s] %s\n", statusIcon, item.ID[:8], priorityLabel, item.Title)
			}

			fmt.Printf("\n%d items\n", len(items))
			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status (new, reviewed, ready, done, ignored)")
	cmd.Flags().StringVar(&priority, "priority", "", "Filter by priority (low, medium, high)")
	cmd.Flags().StringVar(&category, "category", "", "Filter by category")
	cmd.Flags().StringVar(&repo, "repo", "", "Filter by repo")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return cmd
}

func newBacklogShowCmd(getClient func() *client.Client) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "show [id]",
		Short: "Show a backlog item with details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			item, err := getClient().GetBacklogItem(args[0])
			if err != nil {
				return fmt.Errorf("failed to get backlog item: %w", err)
			}

			if asJSON {
				data, _ := json.MarshalIndent(item, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("ID:       %s\n", item.ID)
			fmt.Printf("Title:    %s\n", item.Title)
			fmt.Printf("Summary:  %s\n", item.Summary)
			fmt.Printf("Category: %s\n", item.Category)
			fmt.Printf("Priority: %s\n", item.Priority)
			fmt.Printf("Status:   %s\n", item.Status)
			if item.RepoName != nil {
				fmt.Printf("Repo:     %s\n", *item.RepoName)
			}
			fmt.Printf("Created:  %s\n", item.CreatedAt)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func newBacklogPrioritizeCmd(getClient func() *client.Client) *cobra.Command {
	var (
		priority string
		status   string
	)

	validPriorities := map[string]bool{"low": true, "medium": true, "high": true}
	validStatuses := map[string]bool{"new": true, "reviewed": true, "ready": true, "done": true, "ignored": true}

	cmd := &cobra.Command{
		Use:   "prioritize [id]",
		Short: "Set priority and/or status of a backlog item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			updates := map[string]interface{}{}
			if priority != "" {
				if !validPriorities[priority] {
					return fmt.Errorf("invalid priority '%s', valid values: low, medium, high", priority)
				}
				updates["priority"] = priority
			}
			if status != "" {
				if !validStatuses[status] {
					return fmt.Errorf("invalid status '%s', valid values: new, reviewed, ready, done, ignored", status)
				}
				updates["status"] = status
			}
			if len(updates) == 0 {
				return fmt.Errorf("specify --priority and/or --status")
			}

			item, err := getClient().UpdateBacklogItem(args[0], updates)
			if err != nil {
				return fmt.Errorf("failed to update: %w", err)
			}

			fmt.Printf("Updated %s: priority=%s status=%s\n", item.ID[:8], item.Priority, item.Status)
			return nil
		},
	}

	cmd.Flags().StringVar(&priority, "priority", "", "Set priority (low, medium, high)")
	cmd.Flags().StringVar(&status, "status", "", "Set status (new, reviewed, ready, done, ignored)")

	return cmd
}

func statusSymbol(status string) string {
	switch status {
	case "done":
		return "[x]"
	case "ready":
		return "[>]"
	case "reviewed":
		return "[~]"
	case "ignored":
		return "[-]"
	default:
		return "[ ]"
	}
}

func priorityLabel(p string) string {
	switch p {
	case "high":
		return "HIGH"
	case "medium":
		return "MED "
	default:
		return "LOW "
	}
}
