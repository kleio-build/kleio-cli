package commands

import (
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
		branchName   string
		filePath     string
	)

	cmd := &cobra.Command{
		Use:   "decide [decision text]",
		Short: "Log an engineering decision with alternatives and rationale",
		Long:  "Record a decision as a first-class relational capture in Kleio. Decisions are the fastest-decaying, highest-value signal -- they capture WHY choices were made.",
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

			req := &client.RelationalCaptureCreateRequest{
				AuthorType: "human",
				SourceType: "cli",
				Content:    content,
				Decision: &client.DecisionWrite{
					Alternatives: alts,
					Rationale:    rationale,
					Confidence:   confidence,
				},
			}
			if repo != "" {
				req.RepoName = &repo
			}
			if branchName != "" {
				req.BranchName = &branchName
			}
			if filePath != "" {
				req.FilePath = &filePath
			}

			data, err := getClient().CreateRelationalCapture(req)
			if err != nil {
				return fmt.Errorf("failed to log decision: %w", err)
			}

			fmt.Printf("Decision logged via relational path.\n%s\n", string(data))
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&alternatives, "alternative", nil, "Alternative option considered (can specify multiple)")
	cmd.Flags().StringVar(&rationale, "rationale", "", "Why this decision was made (required)")
	cmd.Flags().StringVar(&confidence, "confidence", "medium", "Confidence level: low, medium, high")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository name")
	cmd.Flags().StringVar(&branchName, "branch", "", "Branch name")
	cmd.Flags().StringVar(&filePath, "file", "", "Related file path (e.g. ADR document)")

	return cmd
}
