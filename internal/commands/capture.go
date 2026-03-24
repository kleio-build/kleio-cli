package commands

import (
	"encoding/json"
	"fmt"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/spf13/cobra"
)

func NewCaptureCmd(getClient func() *client.Client) *cobra.Command {
	var (
		repo       string
		branch     string
		file       string
		lineStart  int
		lineEnd    int
		tags       []string
		context_   string
		sourceType string
		asJSON     bool
	)

	cmd := &cobra.Command{
		Use:   "capture [message]",
		Short: "Capture a work item discovered during development",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := &client.CaptureInput{
				Content:    args[0],
				SourceType: sourceType,
				AuthorType: "human",
				SignalType: "work_item",
			}

			if repo != "" {
				input.RepoName = &repo
			}
			if branch != "" {
				input.BranchName = &branch
			}
			if file != "" {
				input.FilePath = &file
			}
			if lineStart > 0 {
				input.LineStart = &lineStart
			}
			if lineEnd > 0 {
				input.LineEnd = &lineEnd
			}
			if context_ != "" {
				input.FreeformContext = &context_
			}
			input.Tags = tags

			result, err := getClient().CreateCapture(input)
			if err != nil {
				return fmt.Errorf("capture failed: %w", err)
			}

			if asJSON {
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Captured: %s\n", result.CaptureID)
			fmt.Printf("Action:   %s\n", result.Action)
			fmt.Printf("Backlog:  %s\n", result.LinkedBacklogItem)
			if result.DedupeConfidence > 0 {
				fmt.Printf("Dedup:    %.0f%%\n", result.DedupeConfidence*100)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "Repository name")
	cmd.Flags().StringVar(&branch, "branch", "", "Branch name")
	cmd.Flags().StringVar(&file, "file", "", "File path")
	cmd.Flags().IntVar(&lineStart, "line", 0, "Line number")
	cmd.Flags().IntVar(&lineEnd, "line-end", 0, "End line number")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "Tags (can be repeated)")
	cmd.Flags().StringVar(&context_, "context", "", "Additional freeform context")
	cmd.Flags().StringVar(&sourceType, "source", "cli", "Source type sent to the API (e.g. cli, agent, ide)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return cmd
}
