package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/ai"
	kleioclient "github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/kleio-build/kleio-cli/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CaptureInput struct {
	Content         string `json:"content" jsonschema:"The work item text to capture (max 4000 chars)"`
	Title           string `json:"title,omitempty" jsonschema:"Optional short title for the backlog item (max 80 chars). If omitted, the server synthesizes one via LLM."`
	RepoName        string `json:"repo_name,omitempty" jsonschema:"Repository name"`
	BranchName      string `json:"branch_name,omitempty" jsonschema:"Branch name"`
	FilePath        string `json:"file_path,omitempty" jsonschema:"Relevant file path"`
	LineStart       int    `json:"line_start,omitempty" jsonschema:"Start line number"`
	FreeformContext string `json:"freeform_context,omitempty" jsonschema:"Additional context (max 8000 chars)"`
	AuthorType      string `json:"author_type,omitempty" jsonschema:"Who is capturing: human or agent"`
	SignalType      string `json:"signal_type,omitempty" jsonschema:"work_item (default, creates backlog). Do NOT use checkpoint or decision — use kleio_checkpoint / kleio_decide instead"`
}

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
	BacklogItemID      string   `json:"backlog_item_id,omitempty" jsonschema:"Optional: link checkpoint to backlog (UUID or KL-N); closes item when configured"`
}

type DecideInput struct {
	Content      string   `json:"content" jsonschema:"Short capture line summarizing the decision (required)"`
	Alternatives []string `json:"alternatives,omitempty" jsonschema:"Alternative options that were considered"`
	Rationale    string   `json:"rationale" jsonschema:"Why this decision was made (required)"`
	Confidence   string   `json:"confidence" jsonschema:"Confidence level: low, medium, or high (required)"`
	RepoName     string   `json:"repo_name,omitempty" jsonschema:"Repository name"`
	BranchName   string   `json:"branch_name,omitempty" jsonschema:"Branch name"`
	FilePath     string   `json:"file_path,omitempty" jsonschema:"Relevant file path (e.g. ADR document)"`
	AuthorType   string   `json:"author_type,omitempty" jsonschema:"human or agent"`
}

type BacklogListInput struct {
	Status     string `json:"status,omitempty" jsonschema:"Filter by status (new, reviewed, ready, in_progress, done, ignored)"`
	Urgency    string `json:"urgency,omitempty" jsonschema:"Filter by urgency (low, medium, high)"`
	Importance string `json:"importance,omitempty" jsonschema:"Filter by importance (low, medium, high)"`
	Category   string `json:"category,omitempty" jsonschema:"Filter by category"`
	Repo       string `json:"repo,omitempty" jsonschema:"Filter by repository name"`
	Search     string `json:"search,omitempty" jsonschema:"Substring match on title and summary"`
	Assignee   string `json:"assignee,omitempty" jsonschema:"Assignee UUID filter, or self with an authenticated MCP session"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Max items to return after filters (0 = no cap)"`
}

type BacklogShowInput struct {
	ID string `json:"id" jsonschema:"Backlog item ID"`
}

type BacklogPrioritizeInput struct {
	ID         string `json:"id" jsonschema:"Backlog item ID (UUID or KL-N)"`
	Urgency    string `json:"urgency,omitempty" jsonschema:"New urgency (low, medium, high)"`
	Importance string `json:"importance,omitempty" jsonschema:"New importance (low, medium, high)"`
	Status     string `json:"status,omitempty" jsonschema:"New status (includes in_progress)"`
	AssigneeID string `json:"assignee_id,omitempty" jsonschema:"UUID, self, none, or clear"`
}

type AskInput struct {
	Question string `json:"question" jsonschema:"The question to ask about your engineering history (required)"`
}

type TraceInput struct {
	Anchor string `json:"anchor" jsonschema:"File path, feature name, or topic to trace (required)"`
	Since  string `json:"since,omitempty" jsonschema:"Time window filter (e.g. 7d, 24h)"`
}

type SessionSummaryInput struct{}

type TextOutput struct {
	Result string `json:"result"`
}

type sessionState struct {
	mu        sync.Mutex
	startedAt time.Time
	toolCalls map[string]int
}

func newSessionState() *sessionState {
	return &sessionState{
		startedAt: time.Now(),
		toolCalls: make(map[string]int),
	}
}

func (s *sessionState) record(toolName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolCalls[toolName]++
}

func (s *sessionState) tally() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	captures := s.toolCalls["kleio_capture"]
	decisions := s.toolCalls["kleio_decide"]
	checkpoints := s.toolCalls["kleio_checkpoint"]
	return fmt.Sprintf("Session: %d captures, %d decisions, %d checkpoints.", captures, decisions, checkpoints)
}

const kleioInstructions = `Kleio records durable engineering signals. You have three write tools:

1. kleio_capture — smart capture path. Use signal_type=work_item (default) for actionable follow-up work (bugs, debt, feature gaps) — only work_item creates backlog items. Do NOT use signal_type=checkpoint or decision here; the API rejects them.

2. kleio_decide — relational decision path. Call when you commit to a direction after comparing alternatives. Required: content, rationale, confidence (low/medium/high). Include alternatives when they exist.

3. kleio_checkpoint — relational checkpoint path. Record what was implemented in a meaningful slice: validation status, files changed, optional caveats/deferred. Required: content, slice_category, slice_status, validation_status. Optional: backlog_item_id (UUID or KL-N) to link and close the backlog item.

Read tools: kleio_backlog_list (filters: search, assignee including self, limit), kleio_backlog_show (UUID or KL-N). Triage: kleio_backlog_prioritize (urgency, importance, status, assignee_id self/none/UUID). Query: kleio_ask. Trace: kleio_trace.

Platform: high-confidence semantic links from checkpoints or merged PRs (>=0.90) and explicit KL-N references in commit messages can auto-close open backlog items.

RULES:
- If you choose a non-trivial direction, log it with kleio_decide BEFORE implementing.
- If you discover follow-up work, log it with kleio_capture.
- After completing a meaningful implementation slice, log it with kleio_checkpoint.
- Before finishing any non-trivial task, verify that required Kleio records were created.
- A change is non-trivial if it changes schema, API shape, architecture, cross-file control flow, generation strategy, or introduces a meaningful tradeoff.
- Do not spam — batch related findings, skip trivial refactors and one-off typos.
- Check kleio_backlog_list before creating work items that might already exist.`

// NewServer creates an MCP server. It accepts a Store (local or cloud) for
// data operations and an optional API client for cloud-only features.
// When apiClient is nil, cloud-only features (prioritize with assignee self,
// session summary with API captures) degrade gracefully.
func NewServer(store kleio.Store, apiClient *kleioclient.Client) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "kleio-mcp", Version: version.Version}, &mcp.ServerOptions{
		Instructions: kleioInstructions,
	})

	session := newSessionState()

	provider, _ := ai.ResolveProvider(ai.LoadConfig())
	eng := engine.New(store, provider)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_capture",
		Description: "Smart capture path. Use signal_type work_item (default, creates/links backlog). Do NOT use checkpoint or decision — use kleio_checkpoint / kleio_decide instead.",
	}, captureHandler(store, session))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_checkpoint",
		Description: "Relational checkpoint path: record what was implemented in a meaningful slice. Required: content, slice_category, slice_status, validation_status. Optional: backlog_item_id (UUID or KL-N).",
	}, checkpointHandler(store, session))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_decide",
		Description: "Relational decision path: record direction chosen after comparing alternatives. Required: content, rationale, confidence (low/medium/high).",
	}, decideHandler(store, session))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_backlog_list",
		Description: "List backlog items with optional filters (search, limit, status/urgency/importance/category/repo).",
	}, backlogListHandler(store, apiClient))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_backlog_show",
		Description: "Show details of a specific backlog item (UUID or KL-N).",
	}, backlogShowHandler(store, apiClient))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_backlog_prioritize",
		Description: "Update urgency, importance, status, and/or assignee_id (UUID, self, none, clear) of a backlog item.",
	}, backlogPrioritizeHandler(store, apiClient))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_session_summary",
		Description: "Show what Kleio signals have been logged this session.",
	}, sessionSummaryHandler(apiClient, session))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_ask",
		Description: "Query Kleio's engineering memory. In local mode, searches local data with heuristics. In cloud mode, uses AI-synthesized answers.",
	}, askHandler(store, eng, apiClient))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kleio_trace",
		Description: "Trace how a file, feature, or topic evolved over time. Returns a chronological timeline of commits and events.",
	}, traceHandler(eng))

	return s
}

func captureHandler(store kleio.Store, session *sessionState) func(ctx context.Context, req *mcp.CallToolRequest, input CaptureInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input CaptureInput) (*mcp.CallToolResult, TextOutput, error) {
		if input.Content == "" {
			return errResult("content is required"), TextOutput{}, nil
		}
		if strings.EqualFold(strings.TrimSpace(input.SignalType), "checkpoint") {
			return errResult(kleioclient.SmartCaptureCheckpointRejectedMessage), TextOutput{}, nil
		}
		if strings.EqualFold(strings.TrimSpace(input.SignalType), "decision") {
			return errResult(kleioclient.SmartCaptureDecisionRejectedMessage), TextOutput{}, nil
		}

		author := "agent"
		if input.AuthorType != "" {
			author = input.AuthorType
		}
		signalType := kleio.SignalTypeWorkItem
		if input.SignalType != "" {
			signalType = input.SignalType
		}

		evt := &kleio.Event{
			Content:         input.Content,
			SignalType:      signalType,
			SourceType:      kleio.SourceTypeAgent,
			AuthorType:      author,
			RepoName:        input.RepoName,
			BranchName:      input.BranchName,
			FilePath:        input.FilePath,
			FreeformContext: input.FreeformContext,
		}

		if err := store.CreateEvent(ctx, evt); err != nil {
			return errResult("Capture failed: " + err.Error()), TextOutput{}, nil
		}

		session.record("kleio_capture")
		data, _ := json.MarshalIndent(evt, "", "  ")
		return nil, TextOutput{Result: string(data) + "\n" + session.tally()}, nil
	}
}

func checkpointHandler(store kleio.Store, session *sessionState) func(ctx context.Context, req *mcp.CallToolRequest, input CheckpointInput) (*mcp.CallToolResult, TextOutput, error) {
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

		sd := map[string]interface{}{
			"slice_category":       input.SliceCategory,
			"slice_status":         input.SliceStatus,
			"validation_status":    input.ValidationStatus,
			"summary_what_changed": what,
		}
		if input.SummaryWhy != "" {
			sd["summary_why"] = input.SummaryWhy
		}
		if len(input.Files) > 0 {
			sd["files"] = input.Files
		}
		if len(input.Caveats) > 0 {
			sd["caveats"] = input.Caveats
		}
		if len(input.Deferred) > 0 {
			sd["deferred"] = input.Deferred
		}
		if input.BacklogItemID != "" {
			sd["backlog_item_id"] = input.BacklogItemID
		}
		sdJSON, _ := json.Marshal(sd)

		author := "agent"
		if input.AuthorType != "" {
			author = input.AuthorType
		}

		evt := &kleio.Event{
			Content:         input.Content,
			SignalType:      kleio.SignalTypeCheckpoint,
			SourceType:      kleio.SourceTypeAgent,
			AuthorType:      author,
			RepoName:        input.RepoName,
			BranchName:      input.BranchName,
			FilePath:        input.FilePath,
			FreeformContext: input.FreeformContext,
			StructuredData:  string(sdJSON),
		}

		if err := store.CreateEvent(ctx, evt); err != nil {
			return errResult("Checkpoint failed: " + err.Error()), TextOutput{}, nil
		}

		session.record("kleio_checkpoint")
		data, _ := json.MarshalIndent(evt, "", "  ")
		return nil, TextOutput{Result: string(data) + "\n" + session.tally()}, nil
	}
}

func decideHandler(store kleio.Store, session *sessionState) func(ctx context.Context, req *mcp.CallToolRequest, input DecideInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input DecideInput) (*mcp.CallToolResult, TextOutput, error) {
		if input.Content == "" {
			return errResult("content is required"), TextOutput{}, nil
		}
		if strings.TrimSpace(input.Rationale) == "" {
			return errResult("rationale is required"), TextOutput{}, nil
		}
		conf := strings.TrimSpace(input.Confidence)
		if conf == "" {
			conf = "medium"
		}
		switch conf {
		case "low", "medium", "high":
		default:
			return errResult("confidence must be low, medium, or high"), TextOutput{}, nil
		}

		alts := input.Alternatives
		if alts == nil {
			alts = []string{}
		}

		sd := map[string]interface{}{
			"alternatives": alts,
			"rationale":    input.Rationale,
			"confidence":   conf,
		}
		sdJSON, _ := json.Marshal(sd)

		author := "agent"
		if input.AuthorType != "" {
			author = input.AuthorType
		}

		evt := &kleio.Event{
			Content:        input.Content,
			SignalType:     kleio.SignalTypeDecision,
			SourceType:     kleio.SourceTypeAgent,
			AuthorType:     author,
			RepoName:       input.RepoName,
			BranchName:     input.BranchName,
			FilePath:       input.FilePath,
			StructuredData: string(sdJSON),
		}

		if err := store.CreateEvent(ctx, evt); err != nil {
			return errResult("Decision failed: " + err.Error()), TextOutput{}, nil
		}

		session.record("kleio_decide")
		data, _ := json.MarshalIndent(evt, "", "  ")
		return nil, TextOutput{Result: string(data) + "\n" + session.tally()}, nil
	}
}

func backlogListHandler(store kleio.Store, apiClient *kleioclient.Client) func(ctx context.Context, req *mcp.CallToolRequest, input BacklogListInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input BacklogListInput) (*mcp.CallToolResult, TextOutput, error) {
		if store.Mode() == kleio.StoreModeCloud && apiClient != nil {
			items, err := apiClient.ListBacklogItems(kleioclient.BacklogListFilters{
				Status: input.Status, Urgency: input.Urgency, Importance: input.Importance,
				Category: input.Category, Repo: input.Repo, Search: input.Search,
				Assignee: input.Assignee, Limit: input.Limit,
			})
			if err != nil {
				if r := authErrResult(err); r != nil {
					return r, TextOutput{}, nil
				}
				return errResult("List failed: " + err.Error()), TextOutput{}, nil
			}
			data, _ := json.MarshalIndent(items, "", "  ")
			return nil, TextOutput{Result: string(data)}, nil
		}

		items, err := store.ListBacklogItems(ctx, kleio.BacklogFilter{
			Status:     input.Status,
			Urgency:    input.Urgency,
			Importance: input.Importance,
			Category:   input.Category,
			RepoName:   input.Repo,
			Search:     input.Search,
			Limit:      input.Limit,
		})
		if err != nil {
			return errResult("List failed: " + err.Error()), TextOutput{}, nil
		}
		data, _ := json.MarshalIndent(items, "", "  ")
		return nil, TextOutput{Result: string(data)}, nil
	}
}

func backlogShowHandler(store kleio.Store, apiClient *kleioclient.Client) func(ctx context.Context, req *mcp.CallToolRequest, input BacklogShowInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input BacklogShowInput) (*mcp.CallToolResult, TextOutput, error) {
		if input.ID == "" {
			return errResult("id is required"), TextOutput{}, nil
		}

		if store.Mode() == kleio.StoreModeCloud && apiClient != nil {
			resolved, err := apiClient.ResolveBacklogItemRef(input.ID)
			if err != nil {
				return errResult(err.Error()), TextOutput{}, nil
			}
			item, err := apiClient.GetBacklogItem(resolved)
			if err != nil {
				if r := authErrResult(err); r != nil {
					return r, TextOutput{}, nil
				}
				return errResult("Show failed: " + err.Error()), TextOutput{}, nil
			}
			data, _ := json.MarshalIndent(item, "", "  ")
			return nil, TextOutput{Result: string(data)}, nil
		}

		item, err := store.GetBacklogItem(ctx, input.ID)
		if err != nil {
			return errResult("Show failed: " + err.Error()), TextOutput{}, nil
		}
		data, _ := json.MarshalIndent(item, "", "  ")
		return nil, TextOutput{Result: string(data)}, nil
	}
}

func backlogPrioritizeHandler(store kleio.Store, apiClient *kleioclient.Client) func(ctx context.Context, req *mcp.CallToolRequest, input BacklogPrioritizeInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input BacklogPrioritizeInput) (*mcp.CallToolResult, TextOutput, error) {
		if input.ID == "" {
			return errResult("id is required"), TextOutput{}, nil
		}

		if store.Mode() == kleio.StoreModeCloud && apiClient != nil {
			updates := map[string]interface{}{}
			if input.Urgency != "" {
				updates["urgency"] = input.Urgency
			}
			if input.Importance != "" {
				updates["importance"] = input.Importance
			}
			if input.Status != "" {
				updates["status"] = input.Status
			}
			if aid := strings.TrimSpace(input.AssigneeID); aid != "" {
				switch strings.ToLower(aid) {
				case "none", "clear":
					updates["assignee_id"] = nil
				case "self":
					me, merr := apiClient.GetMeJSON()
					if merr != nil {
						return errResult("assignee_id self: could not load /api/me: " + merr.Error()), TextOutput{}, nil
					}
					uid, _ := me["id"].(string)
					if strings.TrimSpace(uid) == "" {
						return errResult("assignee_id self: user id not available"), TextOutput{}, nil
					}
					updates["assignee_id"] = uid
				default:
					updates["assignee_id"] = aid
				}
			}
			if len(updates) == 0 {
				return errResult("Specify urgency, importance, status, and/or assignee_id"), TextOutput{}, nil
			}

			resolvedID, err := apiClient.ResolveBacklogItemRef(input.ID)
			if err != nil {
				return errResult(err.Error()), TextOutput{}, nil
			}
			item, err := apiClient.UpdateBacklogItem(resolvedID, updates)
			if err != nil {
				if r := authErrResult(err); r != nil {
					return r, TextOutput{}, nil
				}
				return errResult("Update failed: " + err.Error()), TextOutput{}, nil
			}
			data, _ := json.MarshalIndent(item, "", "  ")
			return nil, TextOutput{Result: string(data)}, nil
		}

		update := &kleio.BacklogItem{}
		if input.Urgency != "" {
			update.Urgency = input.Urgency
		}
		if input.Importance != "" {
			update.Importance = input.Importance
		}
		if input.Status != "" {
			update.Status = input.Status
		}
		if err := store.UpdateBacklogItem(ctx, input.ID, update); err != nil {
			return errResult("Update failed: " + err.Error()), TextOutput{}, nil
		}
		item, err := store.GetBacklogItem(ctx, input.ID)
		if err != nil {
			return errResult("Fetch updated item failed: " + err.Error()), TextOutput{}, nil
		}
		data, _ := json.MarshalIndent(item, "", "  ")
		return nil, TextOutput{Result: string(data)}, nil
	}
}

func sessionSummaryHandler(apiClient *kleioclient.Client, session *sessionState) func(ctx context.Context, req *mcp.CallToolRequest, input SessionSummaryInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input SessionSummaryInput) (*mcp.CallToolResult, TextOutput, error) {
		session.mu.Lock()
		startedAt := session.startedAt
		captures := session.toolCalls["kleio_capture"]
		decisions := session.toolCalls["kleio_decide"]
		checkpoints := session.toolCalls["kleio_checkpoint"]
		session.mu.Unlock()

		duration := time.Since(startedAt).Truncate(time.Second)

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Session duration: %s\n", duration))
		sb.WriteString(fmt.Sprintf("Tool calls: %d captures, %d decisions, %d checkpoints\n", captures, decisions, checkpoints))

		if apiClient != nil {
			apiCaptures, err := apiClient.ListCaptures(kleioclient.ListCapturesOptions{
				CreatedAfter: startedAt.Format(time.RFC3339),
				AuthorType:   "agent",
			})
			if err == nil && len(apiCaptures) > 0 {
				sb.WriteString(fmt.Sprintf("\nCaptures logged this session (%d):\n", len(apiCaptures)))
				for _, cap := range apiCaptures {
					label := cap.SignalType
					if label == "" {
						label = "capture"
					}
					sb.WriteString(fmt.Sprintf("  - [%s] %s\n", label, truncate(cap.Content, 120)))
				}
			} else if err == nil {
				sb.WriteString("\nNo captures logged this session.\n")
			}
		}

		var nudges []string
		if decisions == 0 {
			nudges = append(nudges, "No decisions logged this session. If you chose a direction, consider calling kleio_decide.")
		}
		if checkpoints == 0 {
			nudges = append(nudges, "No checkpoints logged this session. If you completed an implementation slice, consider calling kleio_checkpoint.")
		}
		if len(nudges) > 0 {
			sb.WriteString("\n")
			for _, n := range nudges {
				sb.WriteString(n + "\n")
			}
		}

		return nil, TextOutput{Result: sb.String()}, nil
	}
}

func askHandler(store kleio.Store, eng *engine.Engine, apiClient *kleioclient.Client) func(ctx context.Context, req *mcp.CallToolRequest, input AskInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input AskInput) (*mcp.CallToolResult, TextOutput, error) {
		if input.Question == "" {
			return errResult("question is required"), TextOutput{}, nil
		}

		if store.Mode() == kleio.StoreModeCloud && apiClient != nil {
			result, err := apiClient.AskMemory(input.Question)
			if err != nil {
				if r := authErrResult(err); r != nil {
					return r, TextOutput{}, nil
				}
				return errResult("Query failed: " + err.Error()), TextOutput{}, nil
			}

			var sb strings.Builder
			sb.WriteString(result.Answer)
			if len(result.Sources) > 0 {
				sb.WriteString("\n\n---\nSources:\n")
				for i, src := range result.Sources {
					sb.WriteString(fmt.Sprintf("[%d] (%s) %s", i+1, src.SignalType, truncate(src.Content, 100)))
					if src.RepoName != "" {
						sb.WriteString(fmt.Sprintf(" [%s]", src.RepoName))
					}
					sb.WriteString(fmt.Sprintf(" (%.0f%% match)\n", src.Similarity*100))
				}
			}
			return nil, TextOutput{Result: sb.String()}, nil
		}

		results, err := eng.Search(ctx, input.Question, 10)
		if err != nil {
			return errResult("Search failed: " + err.Error()), TextOutput{}, nil
		}

		if len(results) == 0 {
			msg := "No matching results found locally. For richer answers, connect to Kleio Cloud with `kleio login`."
			return nil, TextOutput{Result: msg}, nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Found %d result(s) matching %q:\n\n", len(results), input.Question))
		for i, r := range results {
			sb.WriteString(fmt.Sprintf("[%d] (%s) %s", i+1, r.Kind, truncate(r.Content, 120)))
			if r.RepoName != "" {
				sb.WriteString(fmt.Sprintf(" [%s]", r.RepoName))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\nFor richer semantic answers, connect to Kleio Cloud with `kleio login`.")

		return nil, TextOutput{Result: sb.String()}, nil
	}
}

func traceHandler(eng *engine.Engine) func(ctx context.Context, req *mcp.CallToolRequest, input TraceInput) (*mcp.CallToolResult, TextOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input TraceInput) (*mcp.CallToolResult, TextOutput, error) {
		if input.Anchor == "" {
			return errResult("anchor is required"), TextOutput{}, nil
		}

		var sinceTime time.Time
		if input.Since != "" {
			if d, err := time.ParseDuration(input.Since); err == nil {
				sinceTime = time.Now().Add(-d)
			}
		}

		entries, err := eng.Timeline(ctx, input.Anchor, sinceTime)
		if err != nil {
			return errResult("Trace failed: " + err.Error()), TextOutput{}, nil
		}

		if len(entries) == 0 {
			return nil, TextOutput{Result: fmt.Sprintf("No timeline entries found for %q.", input.Anchor)}, nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Timeline for %q (%d entries):\n\n", input.Anchor, len(entries)))
		for _, e := range entries {
			ts := e.Timestamp.Format("2006-01-02 15:04")
			sb.WriteString(fmt.Sprintf("  [%s] %s %s\n", e.Kind, ts, e.Summary))
		}

		return nil, TextOutput{Result: sb.String()}, nil
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
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
