package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/spf13/cobra"
)

func NewCheckCmd(getClient func() *client.Client) *cobra.Command {
	var (
		base     string
		repo     string
		diffPath string
		jsonOut  bool
		mode     string
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run diff change analysis against workspace history",
		Long:  "Analyzes a git diff against Kleio's captured decisions, checkpoints, and backlog to surface relevant context before merging.",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()

			var diff string
			if diffPath == "-" || diffPath == "" {
				stat, _ := os.Stdin.Stat()
				if (stat.Mode() & os.ModeCharDevice) == 0 {
					raw, err := io.ReadAll(os.Stdin)
					if err != nil {
						return fmt.Errorf("reading stdin: %w", err)
					}
					diff = string(raw)
				}
			} else {
				raw, err := os.ReadFile(diffPath)
				if err != nil {
					return fmt.Errorf("reading diff file: %w", err)
				}
				diff = string(raw)
			}

			if diff == "" {
				return fmt.Errorf("no diff provided; pipe a diff via stdin or use --diff")
			}

			if mode == "" {
				mode = "lightweight"
			}

			result, err := c.Check(repo, base, diff, mode)
			if err != nil {
				return err
			}

			if jsonOut {
				formatted, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(formatted))
				return nil
			}

			renderTextResult(result)
			return nil
		},
	}

	cmd.Flags().StringVar(&base, "base", "main", "Base ref for diff comparison")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository name (owner/name)")
	cmd.Flags().StringVar(&diffPath, "diff", "-", "Path to diff file or - for stdin")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output raw JSON")
	cmd.Flags().StringVar(&mode, "mode", "lightweight", "Analysis mode: lightweight or full")

	return cmd
}

func renderTextResult(result json.RawMessage) {
	var parsed struct {
		Summary struct {
			Intent string `json:"intent"`
			RiskScore struct {
				Value   float64  `json:"value"`
				Drivers []string `json:"drivers"`
			} `json:"risk_score"`
		} `json:"summary"`
		Findings []struct {
			Action      string `json:"action"`
			Title       string `json:"title"`
			Explanation string `json:"explanation"`
		} `json:"findings"`
		UntrackedAreasSummary string `json:"untracked_areas_summary"`
		Cost struct {
			EstimatedUSD float64 `json:"estimated_usd"`
			Truncated    bool    `json:"truncated"`
		} `json:"cost"`
	}

	if err := json.Unmarshal(result, &parsed); err != nil {
		fmt.Println(string(result))
		return
	}

	if parsed.Summary.Intent != "" {
		fmt.Printf("Intent: %s\n\n", parsed.Summary.Intent)
	}

	if parsed.Summary.RiskScore.Value > 0 {
		fmt.Printf("Risk: %.0f%%\n", parsed.Summary.RiskScore.Value*100)
	}

	if len(parsed.Findings) == 0 {
		fmt.Println("No significant findings.")
	} else {
		for _, f := range parsed.Findings {
			icon := "[" + f.Action + "]"
			fmt.Printf("%s %s\n  %s\n\n", icon, f.Title, f.Explanation)
		}
	}

	if parsed.UntrackedAreasSummary != "" {
		fmt.Printf("Coverage: %s\n", parsed.UntrackedAreasSummary)
	}

	if parsed.Cost.Truncated {
		fmt.Println("Note: Analysis was truncated due to size or cost limits.")
	}
}
