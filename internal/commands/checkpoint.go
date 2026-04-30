package commands

import (
	"context"
	"encoding/json"
	"fmt"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/spf13/cobra"
)

type checkpointData struct {
	SliceCategory      string   `json:"slice_category"`
	SliceStatus        string   `json:"slice_status"`
	ValidationStatus   string   `json:"validation_status"`
	SummaryWhatChanged string   `json:"summary_what_changed"`
	SummaryWhy         string   `json:"summary_why,omitempty"`
	HandoffBody        string   `json:"handoff_body,omitempty"`
	ValidationNotes    string   `json:"validation_notes,omitempty"`
	ApiImplications    string   `json:"api_implications,omitempty"`
	SchemaImplications string   `json:"schema_implications,omitempty"`
	Files              []string `json:"files,omitempty"`
	Caveats            []string `json:"caveats,omitempty"`
	Deferred           []string `json:"deferred,omitempty"`
	BacklogItemID      string   `json:"backlog_item_id,omitempty"`
}

func NewCheckpointCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		sliceCategory    string
		sliceStatus      string
		validationStatus string
		summaryWhat      string
		summaryWhy       string
		handoff          string
		validationNotes  string
		apiImpl          string
		schemaImpl       string
		repo             string
		branch           string
		filePath         string
		freeform         string
		sourceType       string
		authorType       string
		files            []string
		caveats          []string
		deferred         []string
		asJSON           bool
		backlogItemID    string
	)

	cmd := &cobra.Command{
		Use:   "checkpoint [content]",
		Short: "Create a checkpoint capture (implementation summary)",
		Long: `Creates a capture with a nested checkpoint object.

Requires --slice-category, --slice-status, --validation-status.
The positional argument is a short summary line.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if sliceCategory == "" || sliceStatus == "" || validationStatus == "" {
				return fmt.Errorf("--slice-category, --slice-status, and --validation-status are required")
			}
			content := args[0]
			what := summaryWhat
			if what == "" {
				what = content
			}

			cp := checkpointData{
				SliceCategory:      sliceCategory,
				SliceStatus:        sliceStatus,
				ValidationStatus:   validationStatus,
				SummaryWhatChanged: what,
				SummaryWhy:         summaryWhy,
				HandoffBody:        handoff,
				ValidationNotes:    validationNotes,
				ApiImplications:    apiImpl,
				SchemaImplications: schemaImpl,
				Caveats:            caveats,
				Deferred:           deferred,
				BacklogItemID:      backlogItemID,
			}
			for _, p := range files {
				if p != "" {
					cp.Files = append(cp.Files, p)
				}
			}

			sdJSON, _ := json.Marshal(cp)

			evt := &kleio.Event{
				SignalType:      kleio.SignalTypeCheckpoint,
				Content:         content,
				SourceType:      sourceType,
				AuthorType:      authorType,
				RepoName:        repo,
				BranchName:      branch,
				FilePath:        filePath,
				FreeformContext: freeform,
				StructuredData:  string(sdJSON),
			}

			store := getStore()
			if err := store.CreateEvent(context.Background(), evt); err != nil {
				return fmt.Errorf("checkpoint failed: %w", err)
			}

			if asJSON {
				data, _ := json.MarshalIndent(map[string]string{"id": evt.ID}, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Checkpoint created: %s\n", evt.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&sliceCategory, "slice-category", "", "Checkpoint slice_category (required)")
	cmd.Flags().StringVar(&sliceStatus, "slice-status", "", "Checkpoint slice_status (required)")
	cmd.Flags().StringVar(&validationStatus, "validation-status", "", "Checkpoint validation_status (required)")
	cmd.Flags().StringVar(&summaryWhat, "what-changed", "", "summary_what_changed (defaults to content if omitted)")
	cmd.Flags().StringVar(&summaryWhy, "why", "", "summary_why")
	cmd.Flags().StringVar(&handoff, "handoff", "", "handoff_body")
	cmd.Flags().StringVar(&validationNotes, "validation-notes", "", "validation_notes")
	cmd.Flags().StringVar(&apiImpl, "api-implications", "", "api_implications")
	cmd.Flags().StringVar(&schemaImpl, "schema-implications", "", "schema_implications")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository name")
	cmd.Flags().StringVar(&branch, "branch", "", "Branch name")
	cmd.Flags().StringVar(&filePath, "file", "", "Relevant file path")
	cmd.Flags().StringVar(&freeform, "context", "", "Additional freeform context")
	cmd.Flags().StringVar(&sourceType, "source", "cli", "source_type")
	cmd.Flags().StringVar(&authorType, "author", "human", "author_type (human or agent)")
	cmd.Flags().StringSliceVar(&files, "checkpoint-file", nil, "File path changed in this slice (repeatable)")
	cmd.Flags().StringSliceVar(&caveats, "caveat", nil, "Caveat line (repeatable)")
	cmd.Flags().StringSliceVar(&deferred, "deferred", nil, "Deferred follow-up line (repeatable)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print JSON output")
	cmd.Flags().StringVar(&backlogItemID, "backlog-item-id", "", "Link to backlog item (UUID or KL-N)")

	return cmd
}
