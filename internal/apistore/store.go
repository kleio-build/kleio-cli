package apistore

import (
	"context"
	"encoding/json"
	"fmt"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/client"
)

// Store implements kleio.Store by delegating to the existing HTTP API client.
// Operations that don't have a cloud equivalent return not-supported errors.
type Store struct {
	client *client.Client
}

// New wraps an existing API client as a kleio.Store.
func New(c *client.Client) *Store {
	return &Store{client: c}
}

// Client exposes the underlying API client for operations that need direct
// access (e.g. relational captures, OAuth flows).
func (s *Store) Client() *client.Client { return s.client }

func (s *Store) Mode() kleio.StoreMode { return kleio.StoreModeCloud }

func (s *Store) Close() error { return nil }

// --- Events ---

func (s *Store) CreateEvent(_ context.Context, e *kleio.Event) error {
	if e.SignalType == kleio.SignalTypeCheckpoint || e.SignalType == kleio.SignalTypeDecision {
		return s.createRelationalEvent(e)
	}
	return s.createSmartEvent(e)
}

func (s *Store) createSmartEvent(e *kleio.Event) error {
	input := &client.CaptureInput{
		Content:    e.Content,
		SignalType: e.SignalType,
		SourceType: e.SourceType,
		AuthorType: e.AuthorType,
	}
	if e.RepoName != "" {
		input.RepoName = &e.RepoName
	}
	if e.BranchName != "" {
		input.BranchName = &e.BranchName
	}
	if e.FilePath != "" {
		input.FilePath = &e.FilePath
	}
	if e.FreeformContext != "" {
		input.FreeformContext = &e.FreeformContext
	}
	if e.StructuredData != "" {
		input.StructuredData = &e.StructuredData
	}
	if e.AuthorLabel != "" {
		input.AuthorLabel = &e.AuthorLabel
	}

	result, err := s.client.CreateCapture(input)
	if err != nil {
		return err
	}
	if e.ID == "" {
		e.ID = result.CaptureID
	}
	return nil
}

func (s *Store) createRelationalEvent(e *kleio.Event) error {
	req := &client.RelationalCaptureCreateRequest{
		AuthorType: e.AuthorType,
		SourceType: e.SourceType,
		Content:    e.Content,
	}
	if e.RepoName != "" {
		req.RepoName = &e.RepoName
	}
	if e.BranchName != "" {
		req.BranchName = &e.BranchName
	}
	if e.FilePath != "" {
		req.FilePath = &e.FilePath
	}
	if e.FreeformContext != "" {
		req.FreeformContext = &e.FreeformContext
	}
	if e.StructuredData != "" {
		req.StructuredData = &e.StructuredData
	}

	if e.SignalType == kleio.SignalTypeCheckpoint {
		var cp client.CheckpointWrite
		if err := json.Unmarshal([]byte(e.StructuredData), &cp); err == nil && cp.SliceCategory != "" {
			req.Checkpoint = &cp
			req.StructuredData = nil
		}
	} else if e.SignalType == kleio.SignalTypeDecision {
		var dec client.DecisionWrite
		if err := json.Unmarshal([]byte(e.StructuredData), &dec); err == nil && dec.Rationale != "" {
			req.Decision = &dec
			req.StructuredData = nil
		}
	}

	data, err := s.client.CreateRelationalCapture(req)
	if err != nil {
		return err
	}
	if e.ID == "" {
		var wrap struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(data, &wrap) == nil && wrap.ID != "" {
			e.ID = wrap.ID
		}
	}
	return nil
}

func (s *Store) ListEvents(_ context.Context, f kleio.EventFilter) ([]kleio.Event, error) {
	opts := client.ListCapturesOptions{
		SignalType:   f.SignalType,
		AuthorType:   f.AuthorType,
		CreatedAfter: f.CreatedAfter,
	}
	items, err := s.client.ListCaptures(opts)
	if err != nil {
		return nil, err
	}

	events := make([]kleio.Event, 0, len(items))
	for _, item := range items {
		e := kleio.Event{
			ID:         item.ID,
			Content:    item.Content,
			SignalType: item.SignalType,
			AuthorType: item.AuthorType,
			CreatedAt:  item.CreatedAt,
		}
		if item.RepoName != nil {
			e.RepoName = *item.RepoName
		}
		events = append(events, e)
	}
	return events, nil
}

func (s *Store) GetEvent(_ context.Context, id string) (*kleio.Event, error) {
	raw, err := s.client.GetCaptureDetail(id)
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse capture detail: %w", err)
	}
	e := &kleio.Event{ID: id}
	if v, ok := data["content"].(string); ok {
		e.Content = v
	}
	if v, ok := data["signal_type"].(string); ok {
		e.SignalType = v
	}
	if v, ok := data["source_type"].(string); ok {
		e.SourceType = v
	}
	if v, ok := data["author_type"].(string); ok {
		e.AuthorType = v
	}
	if v, ok := data["created_at"].(string); ok {
		e.CreatedAt = v
	}
	if v, ok := data["repo_name"].(string); ok {
		e.RepoName = v
	}
	return e, nil
}

// --- Backlog ---

func (s *Store) CreateBacklogItem(_ context.Context, item *kleio.BacklogItem) error {
	input := &client.CaptureInput{
		Content:    item.Title,
		SignalType: kleio.SignalTypeWorkItem,
		SourceType: kleio.SourceTypeCLI,
		AuthorType: kleio.AuthorTypeHuman,
	}
	if item.RepoName != "" {
		input.RepoName = &item.RepoName
	}
	result, err := s.client.CreateCapture(input)
	if err != nil {
		return err
	}
	if item.ID == "" {
		item.ID = result.CaptureID
	}
	return nil
}

func (s *Store) ListBacklogItems(_ context.Context, f kleio.BacklogFilter) ([]kleio.BacklogItem, error) {
	apiFilter := client.BacklogListFilters{
		Status:     f.Status,
		Urgency:    f.Urgency,
		Importance: f.Importance,
		Category:   f.Category,
		Repo:       f.RepoName,
		Search:     f.Search,
		Assignee:   f.Assignee,
		Limit:      f.Limit,
	}
	items, err := s.client.ListBacklogItems(apiFilter)
	if err != nil {
		return nil, err
	}

	out := make([]kleio.BacklogItem, 0, len(items))
	for _, item := range items {
		bi := kleio.BacklogItem{
			ID:         item.ID,
			Title:      item.Title,
			Summary:    item.Summary,
			Status:     item.Status,
			Category:   item.Category,
			Urgency:    item.Urgency,
			Importance: item.Importance,
			CreatedAt:  item.CreatedAt,
			UpdatedAt:  item.UpdatedAt,
		}
		if item.ShortID != nil {
			bi.ShortID = *item.ShortID
		}
		if item.RepoName != nil {
			bi.RepoName = *item.RepoName
		}
		out = append(out, bi)
	}
	return out, nil
}

func (s *Store) GetBacklogItem(_ context.Context, id string) (*kleio.BacklogItem, error) {
	item, err := s.client.GetBacklogItem(id)
	if err != nil {
		return nil, err
	}
	bi := &kleio.BacklogItem{
		ID:         item.ID,
		Title:      item.Title,
		Summary:    item.Summary,
		Status:     item.Status,
		Category:   item.Category,
		Urgency:    item.Urgency,
		Importance: item.Importance,
		CreatedAt:  item.CreatedAt,
		UpdatedAt:  item.UpdatedAt,
	}
	if item.ShortID != nil {
		bi.ShortID = *item.ShortID
	}
	if item.RepoName != nil {
		bi.RepoName = *item.RepoName
	}
	return bi, nil
}

func (s *Store) UpdateBacklogItem(_ context.Context, id string, update *kleio.BacklogItem) error {
	fields := make(map[string]interface{})
	if update.Status != "" {
		fields["status"] = update.Status
	}
	if update.Urgency != "" {
		fields["urgency"] = update.Urgency
	}
	if update.Importance != "" {
		fields["importance"] = update.Importance
	}
	if update.Title != "" {
		fields["title"] = update.Title
	}
	if len(fields) == 0 {
		return nil
	}
	_, err := s.client.UpdateBacklogItem(id, fields)
	return err
}

// --- Git Index (not supported in cloud mode) ---

func (s *Store) IndexCommits(_ context.Context, _ string, _ []kleio.Commit) error {
	return fmt.Errorf("IndexCommits is not supported in cloud mode — git indexing is local-only")
}

func (s *Store) QueryCommits(_ context.Context, _ kleio.CommitFilter) ([]kleio.Commit, error) {
	return nil, fmt.Errorf("QueryCommits is not supported in cloud mode — git indexing is local-only")
}

// --- Links (not supported in cloud mode) ---

func (s *Store) CreateLink(_ context.Context, _ *kleio.Link) error {
	return fmt.Errorf("CreateLink is not supported in cloud mode")
}

func (s *Store) QueryLinks(_ context.Context, _ kleio.LinkFilter) ([]kleio.Link, error) {
	return nil, fmt.Errorf("QueryLinks is not supported in cloud mode")
}

// --- File History (not supported in cloud mode) ---

func (s *Store) TrackFileChange(_ context.Context, _ *kleio.FileChange) error {
	return fmt.Errorf("TrackFileChange is not supported in cloud mode")
}

func (s *Store) FileHistory(_ context.Context, _ string) ([]kleio.FileChange, error) {
	return nil, fmt.Errorf("FileHistory is not supported in cloud mode")
}

// --- Search ---

func (s *Store) Search(_ context.Context, query string, _ kleio.SearchOpts) ([]kleio.SearchResult, error) {
	resp, err := s.client.AskMemory(query)
	if err != nil {
		return nil, err
	}

	results := make([]kleio.SearchResult, 0, len(resp.Sources))
	for _, src := range resp.Sources {
		results = append(results, kleio.SearchResult{
			ID:         src.CaptureID,
			Kind:       "event",
			Content:    src.Content,
			Score:      src.Similarity,
			CreatedAt:  src.CreatedAt,
			RepoName:   src.RepoName,
			FilePath:   src.FilePath,
			SignalType: src.SignalType,
		})
	}
	return results, nil
}
