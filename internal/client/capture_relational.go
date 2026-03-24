package client

import (
	"encoding/json"
	"fmt"
)

// SmartCaptureCheckpointRejectedMessage must match kleio-app api/internal/services.SmartCaptureCheckpointRejectedMessage.
const SmartCaptureCheckpointRejectedMessage = "signal_type checkpoint cannot use POST /api/captures/smart; use POST /api/captures with a nested checkpoint object, or the `kleio checkpoint` command / MCP tool `kleio_checkpoint`"

// RelationalCaptureCreateRequest is the JSON body for POST /api/captures (nested checkpoint, targets, topics).
// Field names match api/internal/services/capture_metadata.go (CaptureWriteBody).
type RelationalCaptureCreateRequest struct {
	AuthorLabel     *string              `json:"author_label,omitempty"`
	AuthorType      string               `json:"author_type"`
	BranchName      *string              `json:"branch_name,omitempty"`
	Content         string               `json:"content"`
	DiffExcerpt     *string              `json:"diff_excerpt,omitempty"`
	FilePath        *string              `json:"file_path,omitempty"`
	FreeformContext *string              `json:"freeform_context,omitempty"`
	LineEnd         *int                 `json:"line_end,omitempty"`
	LineStart       *int                 `json:"line_start,omitempty"`
	RepoName        *string              `json:"repo_name,omitempty"`
	SignalStrength  string               `json:"signal_strength,omitempty"`
	SignalType      *string              `json:"signal_type,omitempty"`
	SourceType      string               `json:"source_type"`
	StateHint       *string              `json:"state_hint,omitempty"`
	StructuredData  *string              `json:"structured_data,omitempty"`
	WorkLevel       string               `json:"work_level,omitempty"`
	WorkspaceID     string               `json:"workspace_id"`
	Targets         []CaptureTargetWrite `json:"targets,omitempty"`
	Topics          []string             `json:"topics,omitempty"`
	Checkpoint      *CheckpointWrite     `json:"checkpoint,omitempty"`
}

// CaptureTargetWrite is one target row (api/services.CaptureTargetWrite).
type CaptureTargetWrite struct {
	TargetType  string  `json:"target_type"`
	TargetKey   string  `json:"target_key"`
	Label       *string `json:"label,omitempty"`
	ExternalURL *string `json:"external_url,omitempty"`
	SortOrder   *int    `json:"sort_order,omitempty"`
}

// CheckpointWrite matches api/services.CaptureCheckpointWrite JSON tags.
type CheckpointWrite struct {
	SliceCategory      string                `json:"slice_category"`
	SliceStatus        string                `json:"slice_status"`
	ValidationStatus   string                `json:"validation_status"`
	ValidationNotes    *string               `json:"validation_notes,omitempty"`
	SummaryWhatChanged string                `json:"summary_what_changed"`
	SummaryWhy         *string               `json:"summary_why,omitempty"`
	HandoffBody        *string               `json:"handoff_body,omitempty"`
	ApiImplications    *string               `json:"api_implications,omitempty"`
	SchemaImplications *string               `json:"schema_implications,omitempty"`
	Files              []CheckpointFileWrite `json:"files,omitempty"`
	Caveats            []string              `json:"caveats,omitempty"`
	Deferred           []string              `json:"deferred,omitempty"`
}

// CheckpointFileWrite matches api/services.CaptureCheckpointFileWrite.
type CheckpointFileWrite struct {
	Path           string  `json:"path"`
	ComponentLabel *string `json:"component_label,omitempty"`
}

// CreateRelationalCapture POSTs to /api/captures and returns the raw JSON of the `data` object (capture detail).
func (c *Client) CreateRelationalCapture(req *RelationalCaptureCreateRequest) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.WorkspaceID == "" && c.workspaceID != "" {
		req.WorkspaceID = c.workspaceID
	}
	if req.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBody, err := c.doRequest("POST", "/api/captures", body)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(respBody, &wrap); err != nil {
		return nil, fmt.Errorf("failed to parse capture response: %w", err)
	}
	return wrap.Data, nil
}
