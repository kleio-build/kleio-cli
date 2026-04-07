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
		status     string
		urgency    string
		importance string
		category   string
		repo       string
		asJSON     bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List synthesized backlog items",
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := getClient().ListBacklogItems(status, urgency, importance, category, repo)
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
				fmt.Printf("%s %s [%s] %s\n", statusIcon, item.ID[:8], label, item.Title)
			}

			fmt.Printf("\n%d items\n", len(items))
			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status (new, reviewed, ready, done, ignored)")
	cmd.Flags().StringVar(&urgency, "urgency", "", "Filter by urgency (low, medium, high)")
	cmd.Flags().StringVar(&importance, "importance", "", "Filter by importance (low, medium, high)")
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

			fmt.Printf("ID:         %s\n", item.ID)
			fmt.Printf("Title:      %s\n", item.Title)
			fmt.Printf("Summary:    %s\n", item.Summary)
			fmt.Printf("Category:   %s\n", item.Category)
			fmt.Printf("Urgency:    %s\n", item.Urgency)
			fmt.Printf("Importance: %s\n", item.Importance)
			fmt.Printf("Status:     %s\n", item.Status)
			if item.RepoName != nil {
				fmt.Printf("Repo:       %s\n", *item.RepoName)
			}
			fmt.Printf("Created:    %s\n", item.CreatedAt)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func newBacklogPrioritizeCmd(getClient func() *client.Client) *cobra.Command {
	var (
		urgency    string
		importance string
		status     string
	)

	validAxes := map[string]bool{"low": true, "medium": true, "high": true}
	validStatuses := map[string]bool{"new": true, "reviewed": true, "ready": true, "done": true, "ignored": true}

	cmd := &cobra.Command{
		Use:   "prioritize [id]",
		Short: "Set urgency, importance, and/or status of a backlog item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			updates := map[string]interface{}{}
			if urgency != "" {
				if !validAxes[urgency] {
					return fmt.Errorf("invalid urgency '%s', valid values: low, medium, high", urgency)
				}
				updates["urgency"] = urgency
			}
			if importance != "" {
				if !validAxes[importance] {
					return fmt.Errorf("invalid importance '%s', valid values: low, medium, high", importance)
				}
				updates["importance"] = importance
			}
			if status != "" {
				if !validStatuses[status] {
					return fmt.Errorf("invalid status '%s', valid values: new, reviewed, ready, done, ignored", status)
				}
				updates["status"] = status
			}
			if len(updates) == 0 {
				return fmt.Errorf("specify --urgency, --importance, and/or --status")
			}

			item, err := getClient().UpdateBacklogItem(args[0], updates)
			if err != nil {
				return fmt.Errorf("failed to update: %w", err)
			}

			fmt.Printf("Updated %s: urgency=%s importance=%s status=%s\n", item.ID[:8], item.Urgency, item.Importance, item.Status)
			return nil
		},
	}

	cmd.Flags().StringVar(&urgency, "urgency", "", "Set urgency (low, medium, high)")
	cmd.Flags().StringVar(&importance, "importance", "", "Set importance (low, medium, high)")
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
