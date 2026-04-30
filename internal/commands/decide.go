package commands

import (
	"context"
	"encoding/json"
	"fmt"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/spf13/cobra"
)

type decisionData struct {
	Alternatives []string `json:"alternatives"`
	Rationale    string   `json:"rationale"`
	Confidence   string   `json:"confidence"`
}

func NewDecideCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		alternatives []string
		rationale    string
		confidence   string
		repo         string
		branchName   string
		filePath     string
	)

	cmd := &cobra.Command{
		Use:   "decide [decision text]",
		Short: "Log an engineering decision with alternatives and rationale",
		Long:  "Record a decision in Kleio. Decisions capture WHY choices were made.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			content := args[0]
			for _, a := range args[1:] {
				content += " " + a
			}

			if rationale == "" {
				return fmt.Errorf("--rationale is required")
			}

			alts := alternatives
			if alts == nil {
				alts = []string{}
			}

			dec := decisionData{
				Alternatives: alts,
				Rationale:    rationale,
				Confidence:   confidence,
			}
			sdJSON, _ := json.Marshal(dec)

			evt := &kleio.Event{
				SignalType:     kleio.SignalTypeDecision,
				Content:        content,
				SourceType:     kleio.SourceTypeCLI,
				AuthorType:     kleio.AuthorTypeHuman,
				RepoName:       repo,
				BranchName:     branchName,
				FilePath:       filePath,
				StructuredData: string(sdJSON),
			}

			store := getStore()
			if err := store.CreateEvent(context.Background(), evt); err != nil {
				return fmt.Errorf("failed to log decision: %w", err)
			}

			fmt.Printf("Decision logged: %s\n", evt.ID)
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&alternatives, "alternative", nil, "Alternative option considered (can specify multiple)")
	cmd.Flags().StringVar(&rationale, "rationale", "", "Why this decision was made (required)")
	cmd.Flags().StringVar(&confidence, "confidence", "medium", "Confidence level: low, medium, high")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository name")
	cmd.Flags().StringVar(&branchName, "branch", "", "Branch name")
	cmd.Flags().StringVar(&filePath, "file", "", "Related file path")

	return cmd
}
