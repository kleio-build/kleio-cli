package commands

import (
	"fmt"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/spf13/cobra"
)

func NewObserveCmd(getClient func() *client.Client) *cobra.Command {
	var (
		repo       string
		filePath   string
		weakSignal bool
	)

	cmd := &cobra.Command{
		Use:   "observe [observation text]",
		Short: "Record an observation or weak signal",
		Long:  "Capture something noticed during development that doesn't rise to a work item yet -- code smells, emerging patterns, potential issues.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			content := args[0]
			for _, a := range args[1:] {
				content += " " + a
			}

			signalType := "observation"
			if weakSignal {
				signalType = "weak_signal"
			}

			input := &client.CaptureInput{
				Content:    content,
				SourceType: "cli",
				AuthorType: "human",
				SignalType: signalType,
			}
			if repo != "" {
				input.RepoName = &repo
			}
			if filePath != "" {
				input.FilePath = &filePath
			}

			result, err := getClient().CreateCapture(input)
			if err != nil {
				return fmt.Errorf("failed to record observation: %w", err)
			}

			label := "Observation"
			if weakSignal {
				label = "Weak signal"
			}
			fmt.Printf("%s recorded: %s\n", label, result.CaptureID)
			if result.LinkedBacklogItem != "" {
				fmt.Printf("Linked to backlog item: %s (action: %s)\n", result.LinkedBacklogItem, result.Action)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "Repository name")
	cmd.Flags().StringVar(&filePath, "file", "", "Related file path")
	cmd.Flags().BoolVar(&weakSignal, "weak", false, "Mark as a weak signal instead of observation")

	return cmd
}
