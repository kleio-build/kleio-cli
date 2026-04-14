package commands

import (
	"encoding/json"
	"fmt"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/spf13/cobra"
)

func NewCheckpointCmd(getClient func() *client.Client) *cobra.Command {
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
		lineStart        int
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
		Short: "Create a relational checkpoint capture (implementation summary)",
		Long: `Creates a capture via POST /api/captures with a nested checkpoint object.

This is not the smart/backlog path — use "kleio capture" for that.

Requires --slice-category, --slice-status, --validation-status. The positional argument is a short capture line; use --what-changed for summary_what_changed (defaults to the same as content if omitted).`,
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

			req := &client.RelationalCaptureCreateRequest{
				AuthorType: authorType,
				SourceType: sourceType,
				Content:    content,
				Checkpoint: &client.CheckpointWrite{
					SliceCategory:      sliceCategory,
					SliceStatus:        sliceStatus,
					ValidationStatus:   validationStatus,
					SummaryWhatChanged: what,
				},
			}
			if summaryWhy != "" {
				req.Checkpoint.SummaryWhy = &summaryWhy
			}
			if handoff != "" {
				req.Checkpoint.HandoffBody = &handoff
			}
			if validationNotes != "" {
				req.Checkpoint.ValidationNotes = &validationNotes
			}
			if apiImpl != "" {
				req.Checkpoint.ApiImplications = &apiImpl
			}
			if schemaImpl != "" {
				req.Checkpoint.SchemaImplications = &schemaImpl
			}
			for _, p := range files {
				if p == "" {
					continue
				}
				req.Checkpoint.Files = append(req.Checkpoint.Files, client.CheckpointFileWrite{Path: p})
			}
			if len(caveats) > 0 {
				req.Checkpoint.Caveats = caveats
			}
			if len(deferred) > 0 {
				req.Checkpoint.Deferred = deferred
			}
			if repo != "" {
				req.RepoName = &repo
			}
			if branch != "" {
				req.BranchName = &branch
			}
			if filePath != "" {
				req.FilePath = &filePath
			}
			if lineStart > 0 {
				req.LineStart = &lineStart
			}
			if freeform != "" {
				req.FreeformContext = &freeform
			}
			if backlogItemID != "" {
				req.BacklogItemID = backlogItemID
			}

			data, err := getClient().CreateRelationalCapture(req)
			if err != nil {
				return fmt.Errorf("checkpoint failed: %w", err)
			}

			if asJSON {
				var pretty map[string]interface{}
				if err := json.Unmarshal(data, &pretty); err != nil {
					fmt.Println(string(data))
					return nil
				}
				out, _ := json.MarshalIndent(pretty, "", "  ")
				fmt.Println(string(out))
				return nil
			}

			var wrap struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(data, &wrap)
			if wrap.ID != "" {
				fmt.Printf("Checkpoint capture created: %s\n", wrap.ID)
			} else {
				fmt.Println(string(data))
			}
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
	cmd.Flags().IntVar(&lineStart, "line", 0, "Line number")
	cmd.Flags().StringVar(&freeform, "context", "", "Additional freeform context")
	cmd.Flags().StringVar(&sourceType, "source", "cli", "source_type sent to the API")
	cmd.Flags().StringVar(&authorType, "author", "human", "author_type (human or agent)")
	cmd.Flags().StringSliceVar(&files, "checkpoint-file", nil, "File path changed in this slice (repeatable; checkpoint.files[].path)")
	cmd.Flags().StringSliceVar(&caveats, "caveat", nil, "Caveat line (repeatable)")
	cmd.Flags().StringSliceVar(&deferred, "deferred", nil, "Deferred follow-up line (repeatable)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print full capture detail JSON")
	cmd.Flags().StringVar(&backlogItemID, "backlog-item-id", "", "Link to backlog item (UUID or KL-N); sent as backlog_item_id")

	return cmd
}
