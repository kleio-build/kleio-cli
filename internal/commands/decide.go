package commands

import (
	"encoding/json"
	"fmt"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/spf13/cobra"
)

func NewDecideCmd(getClient func() *client.Client) *cobra.Command {
	var (
		alternatives []string
		rationale    string
		confidence   string
		repo         string
		filePath     string
	)

	cmd := &cobra.Command{
		Use:   "decide [decision text]",
		Short: "Log an engineering decision with alternatives and rationale",
		Long:  "Record a decision as a signal in Kleio. Decisions are the fastest-decaying, highest-value signal -- they capture WHY choices were made.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			content := args[0]
			for _, a := range args[1:] {
				content += " " + a
			}

			structuredData, _ := json.Marshal(map[string]interface{}{
				"alternatives": alternatives,
				"rationale":    rationale,
				"confidence":   confidence,
			})
			freeform := string(structuredData)

			input := &client.CaptureInput{
				Content:         content,
				SourceType:      "cli",
				AuthorType:      "human",
				SignalType:      "decision",
				FreeformContext: &freeform,
			}
			if repo != "" {
				input.RepoName = &repo
			}
			if filePath != "" {
				input.FilePath = &filePath
			}

			result, err := getClient().CreateCapture(input)
			if err != nil {
				return fmt.Errorf("failed to log decision: %w", err)
			}

			fmt.Printf("Decision logged: %s\n", result.CaptureID)
			if result.LinkedBacklogItem != "" {
				fmt.Printf("Linked to backlog item: %s (action: %s)\n", result.LinkedBacklogItem, result.Action)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&alternatives, "alternative", nil, "Alternative option considered (can specify multiple)")
	cmd.Flags().StringVar(&rationale, "rationale", "", "Why this decision was made")
	cmd.Flags().StringVar(&confidence, "confidence", "medium", "Confidence level: low, medium, high")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository name")
	cmd.Flags().StringVar(&filePath, "file", "", "Related file path (e.g. ADR document)")

	return cmd
}
