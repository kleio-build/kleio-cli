package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	kleioclient "github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CaptureInput struct {
	Content         string `json:"content" jsonschema:"The work item text to capture"`
	RepoName        string `json:"repo_name,omitempty" jsonschema:"Repository name"`
	BranchName      string `json:"branch_name,omitempty" jsonschema:"Branch name"`
	FilePath        string `json:"file_path,omitempty" jsonschema:"Relevant file path"`
	LineStart       int    `json:"line_start,omitempty" jsonschema:"Start line number"`
	FreeformContext string `json:"freeform_context,omitempty" jsonschema:"Additional context"`
	AuthorType      string `json:"author_type,omitempty" jsonschema:"Who is capturing: human or agent"`
	SignalType      string `json:"signal_type,omitempty" jsonschema:"Smart/backlog path only: work_item, observation, weak_signal, or decision-style via other tools. Do not use checkpoint here — use kleio_checkpoint instead"`
}

// CheckpointInput is the relational checkpoint path (POST /api/captures). Field names match the API checkpoint object.
type CheckpointInput struct {
	Content            string   `json:"content" jsonschema:"Short capture line (required)"`
	SummaryWhatChanged string   `json:"summary_what_changed,omitempty" jsonschema:"What changed in this slice; defaults to content if omitted"`
	SliceCategory      string   `json:"slice_category" jsonschema:"Required: implementation, refactor, bugfix, migration, handoff, or spike"`
	SliceStatus        string   `json:"slice_status" jsonschema:"Required: completed, partial, paused, parked, or handoff_pending"`
	ValidationStatus   string   `json:"validation_status" jsonschema:"Required: not_run, passed, failed, or partial"`
	SummaryWhy         string   `json:"summary_why,omitempty" jsonschema:"Optional: why this slice mattered or context for the change"`
	HandoffBody        string   `json:"handoff_body,omitempty" jsonschema:"Optional: handoff notes for the next owner"`
	ValidationNotes    string   `json:"validation_notes,omitempty" jsonschema:"Optional: how validation was run or what failed"`
	ApiImplications    string   `json:"api_implications,omitempty" jsonschema:"Optional: API surface impact"`
	SchemaImplications string   `json:"schema_implications,omitempty" jsonschema:"Optional: data or schema impact"`
	RepoName           string   `json:"repo_name,omitempty" jsonschema:"Repository name for provenance"`
	BranchName         string   `json:"branch_name,omitempty" jsonschema:"Branch name for provenance"`
	FilePath           string   `json:"file_path,omitempty" jsonschema:"Relevant file path for provenance"`
	LineStart          int      `json:"line_start,omitempty" jsonschema:"Optional: line number in file"`
	FreeformContext    string   `json:"freeform_context,omitempty" jsonschema:"Optional: extra unstructured context"`
	AuthorType         string   `json:"author_type,omitempty" jsonschema:"human or agent"`
	Files              []string `json:"files,omitempty" jsonschema:"Changed file paths (checkpoint.files)"`
	Caveats            []string `json:"caveats,omitempty" jsonschema:"Optional: caveat lines (repeat in array)"`
	Deferred           []string `json:"deferred,omitempty" jsonschema:"Optional: deferred follow-up lines (repeat in array)"`
}

type DecideInput struct {
	Content      string   `json:"content" jsonschema:"The decision that was made"`
	Alternatives []string `json:"alternatives,omitempty" jsonschema:"Alternative options that were considered"`
	Rationale    string   `json:"rationale,omitempty" jsonschema:"Why this decision was made"`
	Confidence   string   `json:"confidence,omitempty" jsonschema:"Confidence level: low, medium, high"`
	RepoName     string   `json:"repo_name,omitempty" jsonschema:"Repository name"`
	FilePath     string   `json:"file_path,omitempty" jsonschema:"Relevant file path (e.g. ADR document)"`
}

type ObserveInput struct {
	Content    string `json:"content" jsonschema:"The observation or weak signal to record"`
	RepoName   string `json:"repo_name,omitempty" jsonschema:"Repository name"`
	FilePath   string `json:"file_path,omitempty" jsonschema:"Relevant file path"`
	SignalType string `json:"signal_type,omitempty" jsonschema:"Signal type: observation or weak_signal (default: observation)"`
}

type BacklogListInput struct {
	Status   string `json:"status,omitempty" jsonschema:"Filter by status (new, reviewed, ready, done, ignored)"`
	Priority string `json:"priority,omitempty" jsonschema:"Filter by priority (low, medium, high)"`
	Category string `json:"category,omitempty" jsonschema:"Filter by category"`
	Repo     string `json:"repo,omitempty" jsonschema:"Filter by repository name"`
}

type BacklogShowInput struct {
	ID string `json:"id" jsonschema:"Backlog item ID"`
}

type BacklogPrioritizeInput struct {
	ID       string `json:"id" jsonschema:"Backlog item ID"`
	Priority string `json:"priority,omitempty" jsonschema:"New priority (low, medium, high)"`
	Status   string `json:"status,omitempty" jsonschema:"New status (new, reviewed, ready, done, ignored)"`
}

type TextOutput struct {
	Result string `json:"result"`
}

func NewServer(apiClient *kleioclient.Client) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "kleio-mcp", Version: version.Version}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_capture",
		Description: "Smart/backlog path only: POST /api/captures/smart. Captures a work item and synthesizes backlog with dedup. Use signal_type work_item, observation, or weak_signal. Do NOT use signal_type checkpoint — use kleio_checkpoint for relational checkpoints.",
	}, captureHandler(apiClient))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_checkpoint",
		Description: "Relational checkpoint path: POST /api/captures with nested checkpoint (implementation summary, provenance). Not for backlog synthesis. Required: content, slice_category, slice_status, validation_status.",
	}, checkpointHandler(apiClient))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_decide",
		Description: "Log an engineering decision with alternatives and rationale. Decisions are the fastest-decaying, highest-value signal -- six months later, nobody remembers WHY approach A was chosen over B.",
	}, decideHandler(apiClient))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_observe",
		Description: "Record an observation or weak signal. Use for things noticed during development that don't rise to a work item yet -- patterns, smells, potential issues.",
	}, observeHandler(apiClient))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_backlog_list",
		Description: "List synthesized backlog items with optional filters.",
	}, backlogListHandler(apiClient))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_backlog_show",
		Description: "Show details of a specific backlog item.",
	}, backlogShowHandler(apiClient))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_backlog_prioritize",
		Description: "Update the priority and/or status of a backlog item.",
	}, backlogPrioritizeHandler(apiClient))

	return s
}

func captureHandler(c *kleioclient.Client) func(ctx context.Context, req *mcp.CallToolRequest, input CaptureInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input CaptureInput) (*mcp.CallToolResult, TextOutput, error) {
		if input.Content == "" {
			return errResult("content is required"), TextOutput{}, nil
		}
		if strings.EqualFold(strings.TrimSpace(input.SignalType), "checkpoint") {
			return errResult(kleioclient.SmartCaptureCheckpointRejectedMessage), TextOutput{}, nil
		}

		captureInput := &kleioclient.CaptureInput{
			Content:    input.Content,
			SourceType: "agent",
			AuthorType: "agent",
		}
		if input.AuthorType != "" {
			captureInput.AuthorType = input.AuthorType
		}
		if input.RepoName != "" {
			captureInput.RepoName = &input.RepoName
		}
		if input.BranchName != "" {
			captureInput.BranchName = &input.BranchName
		}
		if input.FilePath != "" {
			captureInput.FilePath = &input.FilePath
		}
		if input.LineStart > 0 {
			captureInput.LineStart = &input.LineStart
		}
		if input.FreeformContext != "" {
			captureInput.FreeformContext = &input.FreeformContext
		}
		if input.SignalType != "" {
			captureInput.SignalType = input.SignalType
		}

		result, err := c.CreateCapture(captureInput)
		if err != nil {
			if r := authErrResult(err); r != nil {
				return r, TextOutput{}, nil
			}
			return errResult("Capture failed: " + err.Error()), TextOutput{}, nil
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return nil, TextOutput{Result: string(data)}, nil
	}
}

func checkpointHandler(c *kleioclient.Client) func(ctx context.Context, req *mcp.CallToolRequest, input CheckpointInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input CheckpointInput) (*mcp.CallToolResult, TextOutput, error) {
		if input.Content == "" {
			return errResult("content is required"), TextOutput{}, nil
		}
		if strings.TrimSpace(input.SliceCategory) == "" || strings.TrimSpace(input.SliceStatus) == "" || strings.TrimSpace(input.ValidationStatus) == "" {
			return errResult("slice_category, slice_status, and validation_status are required"), TextOutput{}, nil
		}

		what := strings.TrimSpace(input.SummaryWhatChanged)
		if what == "" {
			what = input.Content
		}

		cp := &kleioclient.CheckpointWrite{
			SliceCategory:      strings.TrimSpace(input.SliceCategory),
			SliceStatus:        strings.TrimSpace(input.SliceStatus),
			ValidationStatus:   strings.TrimSpace(input.ValidationStatus),
			SummaryWhatChanged: what,
		}
		if input.SummaryWhy != "" {
			s := input.SummaryWhy
			cp.SummaryWhy = &s
		}
		if input.HandoffBody != "" {
			s := input.HandoffBody
			cp.HandoffBody = &s
		}
		if input.ValidationNotes != "" {
			s := input.ValidationNotes
			cp.ValidationNotes = &s
		}
		if input.ApiImplications != "" {
			s := input.ApiImplications
			cp.ApiImplications = &s
		}
		if input.SchemaImplications != "" {
			s := input.SchemaImplications
			cp.SchemaImplications = &s
		}
		for _, p := range input.Files {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			cp.Files = append(cp.Files, kleioclient.CheckpointFileWrite{Path: p})
		}
		if len(input.Caveats) > 0 {
			cp.Caveats = append(cp.Caveats, input.Caveats...)
		}
		if len(input.Deferred) > 0 {
			cp.Deferred = append(cp.Deferred, input.Deferred...)
		}

		author := "agent"
		if input.AuthorType != "" {
			author = input.AuthorType
		}

		reqBody := &kleioclient.RelationalCaptureCreateRequest{
			AuthorType: author,
			SourceType: "agent",
			Content:    input.Content,
			Checkpoint: cp,
		}
		if input.RepoName != "" {
			reqBody.RepoName = &input.RepoName
		}
		if input.BranchName != "" {
			reqBody.BranchName = &input.BranchName
		}
		if input.FilePath != "" {
			reqBody.FilePath = &input.FilePath
		}
		if input.LineStart > 0 {
			reqBody.LineStart = &input.LineStart
		}
		if input.FreeformContext != "" {
			reqBody.FreeformContext = &input.FreeformContext
		}

		data, err := c.CreateRelationalCapture(reqBody)
		if err != nil {
			if r := authErrResult(err); r != nil {
				return r, TextOutput{}, nil
			}
			return errResult("Checkpoint failed: " + err.Error()), TextOutput{}, nil
		}
		pretty := map[string]interface{}{}
		if err := json.Unmarshal(data, &pretty); err != nil {
			return nil, TextOutput{Result: string(data)}, nil
		}
		out, _ := json.MarshalIndent(pretty, "", "  ")
		return nil, TextOutput{Result: string(out)}, nil
	}
}

func decideHandler(c *kleioclient.Client) func(ctx context.Context, req *mcp.CallToolRequest, input DecideInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input DecideInput) (*mcp.CallToolResult, TextOutput, error) {
		if input.Content == "" {
			return errResult("content is required"), TextOutput{}, nil
		}

		sd, _ := json.Marshal(map[string]interface{}{
			"schema":       "kleio/decision/v1",
			"alternatives": input.Alternatives,
			"rationale":    input.Rationale,
			"confidence":   input.Confidence,
		})
		sdStr := string(sd)

		captureInput := &kleioclient.CaptureInput{
			Content:        input.Content,
			SourceType:     "agent",
			AuthorType:     "agent",
			SignalType:     "decision",
			StructuredData: &sdStr,
		}
		if input.RepoName != "" {
			captureInput.RepoName = &input.RepoName
		}
		if input.FilePath != "" {
			captureInput.FilePath = &input.FilePath
		}

		result, err := c.CreateCapture(captureInput)
		if err != nil {
			if r := authErrResult(err); r != nil {
				return r, TextOutput{}, nil
			}
			return errResult("Decision capture failed: " + err.Error()), TextOutput{}, nil
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return nil, TextOutput{Result: string(data)}, nil
	}
}

func observeHandler(c *kleioclient.Client) func(ctx context.Context, req *mcp.CallToolRequest, input ObserveInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input ObserveInput) (*mcp.CallToolResult, TextOutput, error) {
		if input.Content == "" {
			return errResult("content is required"), TextOutput{}, nil
		}

		signalType := "observation"
		if input.SignalType == "weak_signal" {
			signalType = "weak_signal"
		}

		captureInput := &kleioclient.CaptureInput{
			Content:    input.Content,
			SourceType: "agent",
			AuthorType: "agent",
			SignalType: signalType,
		}
		if input.RepoName != "" {
			captureInput.RepoName = &input.RepoName
		}
		if input.FilePath != "" {
			captureInput.FilePath = &input.FilePath
		}

		result, err := c.CreateCapture(captureInput)
		if err != nil {
			if r := authErrResult(err); r != nil {
				return r, TextOutput{}, nil
			}
			return errResult("Observation capture failed: " + err.Error()), TextOutput{}, nil
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return nil, TextOutput{Result: string(data)}, nil
	}
}

func backlogListHandler(c *kleioclient.Client) func(ctx context.Context, req *mcp.CallToolRequest, input BacklogListInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input BacklogListInput) (*mcp.CallToolResult, TextOutput, error) {
		items, err := c.ListBacklogItems(input.Status, input.Priority, input.Category, input.Repo)
		if err != nil {
			if r := authErrResult(err); r != nil {
				return r, TextOutput{}, nil
			}
			return errResult("List failed: " + err.Error()), TextOutput{}, nil
		}
		data, _ := json.MarshalIndent(items, "", "  ")
		return nil, TextOutput{Result: string(data)}, nil
	}
}

func backlogShowHandler(c *kleioclient.Client) func(ctx context.Context, req *mcp.CallToolRequest, input BacklogShowInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input BacklogShowInput) (*mcp.CallToolResult, TextOutput, error) {
		if input.ID == "" {
			return errResult("id is required"), TextOutput{}, nil
		}
		item, err := c.GetBacklogItem(input.ID)
		if err != nil {
			if r := authErrResult(err); r != nil {
				return r, TextOutput{}, nil
			}
			return errResult("Show failed: " + err.Error()), TextOutput{}, nil
		}
		data, _ := json.MarshalIndent(item, "", "  ")
		return nil, TextOutput{Result: string(data)}, nil
	}
}

func backlogPrioritizeHandler(c *kleioclient.Client) func(ctx context.Context, req *mcp.CallToolRequest, input BacklogPrioritizeInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input BacklogPrioritizeInput) (*mcp.CallToolResult, TextOutput, error) {
		if input.ID == "" {
			return errResult("id is required"), TextOutput{}, nil
		}

		updates := map[string]interface{}{}
		if input.Priority != "" {
			updates["priority"] = input.Priority
		}
		if input.Status != "" {
			updates["status"] = input.Status
		}
		if len(updates) == 0 {
			return errResult("Specify priority and/or status to update"), TextOutput{}, nil
		}

		item, err := c.UpdateBacklogItem(input.ID, updates)
		if err != nil {
			if r := authErrResult(err); r != nil {
				return r, TextOutput{}, nil
			}
			return errResult("Update failed: " + err.Error()), TextOutput{}, nil
		}

		data, _ := json.MarshalIndent(item, "", "  ")
		return nil, TextOutput{Result: string(data)}, nil
	}
}

func authErrResult(err error) *mcp.CallToolResult {
	if errors.Is(err, kleioclient.ErrAuthRequired) {
		return errResult(kleioclient.ErrAuthRequired.Error())
	}
	if strings.Contains(err.Error(), "API error (401)") || strings.Contains(err.Error(), "API error (403)") {
		return errResult("Authentication required. Run `kleio login` in a terminal to authenticate, then restart the MCP server.")
	}
	return nil
}

func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: msg,
			},
		},
	}
}
