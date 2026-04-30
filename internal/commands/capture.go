package commands

import (
	"context"
	"encoding/json"
	"fmt"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/spf13/cobra"
)

func NewCaptureCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		repo       string
		branch     string
		file       string
		context_   string
		sourceType string
		asJSON     bool
	)

	cmd := &cobra.Command{
		Use:   "capture [message]",
		Short: "Capture a work item discovered during development",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			evt := &kleio.Event{
				Content:    args[0],
				SignalType: kleio.SignalTypeWorkItem,
				SourceType: sourceType,
				AuthorType: kleio.AuthorTypeHuman,
				RepoName:   repo,
				BranchName: branch,
				FilePath:   file,
				FreeformContext: context_,
			}

			store := getStore()
			if err := store.CreateEvent(context.Background(), evt); err != nil {
				return fmt.Errorf("capture failed: %w", err)
			}

			if asJSON {
				data, _ := json.MarshalIndent(map[string]string{
					"id":     evt.ID,
					"action": "created",
				}, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Captured: %s\n", evt.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "Repository name")
	cmd.Flags().StringVar(&branch, "branch", "", "Branch name")
	cmd.Flags().StringVar(&file, "file", "", "File path")
	cmd.Flags().StringVar(&context_, "context", "", "Additional freeform context")
	cmd.Flags().StringVar(&sourceType, "source", "cli", "Source type (e.g. cli, agent, ide)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return cmd
}
