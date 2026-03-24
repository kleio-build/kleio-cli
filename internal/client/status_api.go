package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WorkspaceCounts is returned by GET /api/workspace/counts.
type WorkspaceCounts struct {
	WorkspaceID       string           `json:"workspace_id"`
	WorkspaceName     string           `json:"workspace_name"`
	WorkspaceSlug     string           `json:"workspace_slug"`
	CapturesCount     int64            `json:"captures_count"`
	BacklogItemsCount int64            `json:"backlog_items_count"`
	BacklogByStatus   map[string]int64 `json:"backlog_by_status"`
}

// HealthCheck calls GET /api/health without authentication.
func HealthCheck(baseURL string, httpClient *http.Client) error {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	u := strings.TrimSuffix(strings.TrimSpace(baseURL), "/") + "/api/health"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// GetMeJSON returns the parsed /api/me "data" object.
func (c *Client) GetMeJSON() (map[string]interface{}, error) {
	body, err := c.doRequest("GET", "/api/me", nil)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("parse /api/me: %w", err)
	}
	var m map[string]interface{}
	if len(wrapper.Data) > 0 {
		if err := json.Unmarshal(wrapper.Data, &m); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// GetWorkspaceCounts calls GET /api/workspace/counts (requires auth + X-Workspace-ID).
func (c *Client) GetWorkspaceCounts() (*WorkspaceCounts, error) {
	body, err := c.doRequest("GET", "/api/workspace/counts", nil)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("parse workspace counts: %w", err)
	}
	var out WorkspaceCounts
	if len(wrapper.Data) > 0 {
		if err := json.Unmarshal(wrapper.Data, &out); err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// GetWorkspaceCountsRaw returns the raw HTTP body from GET /api/workspace/counts.
func (c *Client) GetWorkspaceCountsRaw() ([]byte, error) {
	return c.doRequest("GET", "/api/workspace/counts", nil)
}
