package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/localdb"
)

// SyncResult reports what a push/pull operation did.
type SyncResult struct {
	EventsPushed      int `json:"events_pushed"`
	BacklogPushed     int `json:"backlog_pushed"`
	EventsPulled      int `json:"events_pulled"`
	BacklogPulled     int `json:"backlog_pulled"`
	Errors            int `json:"errors"`
}

// Syncer handles bidirectional sync between local SQLite and the cloud API.
type Syncer struct {
	local     *localdb.Store
	apiClient *client.Client
}

// New creates a Syncer from a local store and an authenticated API client.
func New(local *localdb.Store, apiClient *client.Client) *Syncer {
	return &Syncer{local: local, apiClient: apiClient}
}

// Push uploads all unsynced events and backlog items to the cloud, then
// marks them as synced locally.
func (s *Syncer) Push(ctx context.Context) (*SyncResult, error) {
	result := &SyncResult{}

	events, err := s.local.ListUnsyncedEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsynced events: %w", err)
	}

	for _, ev := range events {
		if pushErr := s.pushEvent(ev); pushErr != nil {
			result.Errors++
			continue
		}
		if markErr := s.local.MarkEventSynced(ctx, ev.ID); markErr != nil {
			result.Errors++
			continue
		}
		result.EventsPushed++
	}

	items, err := s.local.ListUnsyncedBacklogItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsynced backlog: %w", err)
	}

	for _, item := range items {
		if pushErr := s.pushBacklogItem(item); pushErr != nil {
			result.Errors++
			continue
		}
		if markErr := s.local.MarkBacklogItemSynced(ctx, item.ID); markErr != nil {
			result.Errors++
			continue
		}
		result.BacklogPushed++
	}

	return result, nil
}

// Pull downloads recent cloud data into the local store.
func (s *Syncer) Pull(ctx context.Context) (*SyncResult, error) {
	result := &SyncResult{}

	captures, err := s.apiClient.ListCaptures(client.ListCapturesOptions{
		CreatedAfter: time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("pull captures: %w", err)
	}

	for _, cap := range captures {
		evt := &kleio.Event{
			ID:         cap.ID,
			SignalType: cap.SignalType,
			Content:    cap.Content,
			SourceType: kleio.SourceTypeAPI,
			CreatedAt:  cap.CreatedAt,
			RepoName:   derefStr(cap.RepoName),
			AuthorType: cap.AuthorType,
			Synced:     true,
		}
		if err := s.local.CreateEvent(ctx, evt); err != nil {
			continue
		}
		result.EventsPulled++
	}

	apiItems, err := s.apiClient.ListBacklogItems(client.BacklogListFilters{})
	if err != nil {
		return nil, fmt.Errorf("pull backlog: %w", err)
	}

	for _, apiItem := range apiItems {
		item := &kleio.BacklogItem{
			ID:         apiItem.ID,
			Title:      apiItem.Title,
			Summary:    apiItem.Summary,
			Status:     apiItem.Status,
			Category:   apiItem.Category,
			Urgency:    apiItem.Urgency,
			Importance: apiItem.Importance,
			RepoName:   derefStr(apiItem.RepoName),
			CreatedAt:  apiItem.CreatedAt,
			UpdatedAt:  apiItem.UpdatedAt,
			Synced:     true,
		}
		if err := s.local.CreateBacklogItem(ctx, item); err != nil {
			continue
		}
		result.BacklogPulled++
	}

	return result, nil
}

func (s *Syncer) pushEvent(ev kleio.Event) error {
	if ev.SignalType == kleio.SignalTypeCheckpoint || ev.SignalType == kleio.SignalTypeDecision {
		return s.pushRelationalEvent(ev)
	}
	return s.pushSmartEvent(ev)
}

func (s *Syncer) pushSmartEvent(ev kleio.Event) error {
	captureInput := &client.CaptureInput{
		Content:    ev.Content,
		SourceType: ev.SourceType,
		AuthorType: ev.AuthorType,
	}
	if ev.SignalType != "" {
		captureInput.SignalType = ev.SignalType
	}
	if ev.RepoName != "" {
		captureInput.RepoName = &ev.RepoName
	}
	if ev.BranchName != "" {
		captureInput.BranchName = &ev.BranchName
	}
	if ev.FilePath != "" {
		captureInput.FilePath = &ev.FilePath
	}
	if ev.FreeformContext != "" {
		captureInput.FreeformContext = &ev.FreeformContext
	}

	_, err := s.apiClient.CreateCapture(captureInput)
	return err
}

func (s *Syncer) pushRelationalEvent(ev kleio.Event) error {
	reqBody := &client.RelationalCaptureCreateRequest{
		AuthorType: ev.AuthorType,
		SourceType: ev.SourceType,
		Content:    ev.Content,
	}
	if ev.RepoName != "" {
		reqBody.RepoName = &ev.RepoName
	}
	if ev.BranchName != "" {
		reqBody.BranchName = &ev.BranchName
	}
	if ev.FilePath != "" {
		reqBody.FilePath = &ev.FilePath
	}
	if ev.FreeformContext != "" {
		reqBody.FreeformContext = &ev.FreeformContext
	}

	if ev.SignalType == kleio.SignalTypeCheckpoint {
		var cp client.CheckpointWrite
		if ev.StructuredData != "" {
			_ = json.Unmarshal([]byte(ev.StructuredData), &cp)
		}
		reqBody.Checkpoint = &cp
	} else if ev.SignalType == kleio.SignalTypeDecision {
		var dec client.DecisionWrite
		if ev.StructuredData != "" {
			_ = json.Unmarshal([]byte(ev.StructuredData), &dec)
		}
		reqBody.Decision = &dec
	}

	_, err := s.apiClient.CreateRelationalCapture(reqBody)
	return err
}

func (s *Syncer) pushBacklogItem(item kleio.BacklogItem) error {
	captureInput := &client.CaptureInput{
		Content:    item.Title + ": " + item.Summary,
		SourceType: "cli",
		AuthorType: "human",
		SignalType: kleio.SignalTypeWorkItem,
	}
	if item.RepoName != "" {
		captureInput.RepoName = &item.RepoName
	}
	_, err := s.apiClient.CreateCapture(captureInput)
	return err
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
