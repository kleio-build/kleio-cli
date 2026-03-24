package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	baseURL     string
	apiKey      string
	token       string
	workspaceID string
	httpClient  *http.Client
}

// New creates a client that uses X-API-Key auth. workspaceID is sent on every request
// as X-Workspace-ID when non-empty (required for most /api routes).
func New(baseURL, apiKey, workspaceID string) *Client {
	return &Client{
		baseURL:     baseURL,
		apiKey:      apiKey,
		workspaceID: workspaceID,
		httpClient:  &http.Client{},
	}
}

func NewWithToken(baseURL, token, workspaceID string) *Client {
	return &Client{
		baseURL:     baseURL,
		token:       token,
		workspaceID: workspaceID,
		httpClient:  &http.Client{},
	}
}

type CaptureInput struct {
	Content         string   `json:"content"`
	SourceType      string   `json:"source_type"`
	RepoName        *string  `json:"repo_name,omitempty"`
	BranchName      *string  `json:"branch_name,omitempty"`
	FilePath        *string  `json:"file_path,omitempty"`
	LineStart       *int     `json:"line_start,omitempty"`
	LineEnd         *int     `json:"line_end,omitempty"`
	FreeformContext *string  `json:"freeform_context,omitempty"`
	DiffExcerpt     *string  `json:"diff_excerpt,omitempty"`
	AuthorType      string   `json:"author_type"`
	AuthorLabel     *string  `json:"author_label,omitempty"`
	WorkspaceID     string   `json:"workspace_id,omitempty"`
	SignalType      string   `json:"signal_type,omitempty"`
	StructuredData  *string  `json:"structured_data,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

type CaptureResult struct {
	CaptureID         string  `json:"capture_id"`
	LinkedBacklogItem string  `json:"linked_backlog_item_id"`
	Action            string  `json:"action"`
	DedupeConfidence  float64 `json:"dedupe_confidence"`
}

type BacklogItem struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Summary          string   `json:"summary"`
	Category         string   `json:"category"`
	Priority         string   `json:"priority"`
	Status           string   `json:"status"`
	RepoName         *string  `json:"repo_name"`
	DedupeConfidence *float64 `json:"dedupe_confidence"`
	WorkspaceID      string   `json:"workspace_id"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

type Workspace struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Slug             string  `json:"slug"`
	GitHubOwnerLogin *string `json:"git_hub_owner_login"`
	Plan             string  `json:"plan"`
}

func (c *Client) ExchangeCode(code string) (*TokenResponse, error) {
	body, _ := json.Marshal(map[string]string{"code": code})
	resp, err := c.doRequest("POST", "/auth/token", body)
	if err != nil {
		return nil, err
	}
	var token TokenResponse
	if err := json.Unmarshal(resp, &token); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	return &token, nil
}

func (c *Client) ListWorkspaces() ([]Workspace, error) {
	resp, err := c.doRequest("GET", "/api/me/workspaces", nil)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Data []Workspace `json:"data"`
	}
	if err := json.Unmarshal(resp, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return wrapper.Data, nil
}

func (c *Client) CreateCapture(input *CaptureInput) (*CaptureResult, error) {
	if input.WorkspaceID == "" && c.workspaceID != "" {
		input.WorkspaceID = c.workspaceID
	}
	if input.SignalType == "" {
		input.SignalType = "work_item"
	}
	if strings.EqualFold(strings.TrimSpace(input.SignalType), "checkpoint") {
		return nil, fmt.Errorf("%s", SmartCaptureCheckpointRejectedMessage)
	}
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest("POST", "/api/captures/smart", body)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Data CaptureResult `json:"data"`
	}
	if err := json.Unmarshal(resp, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &wrapper.Data, nil
}

func (c *Client) ListBacklogItems(status, priority, category, repo string) ([]BacklogItem, error) {
	params := url.Values{}
	if status != "" {
		params.Set("status", status)
	}
	if priority != "" {
		params.Set("priority", priority)
	}
	if category != "" {
		params.Set("category", category)
	}
	if repo != "" {
		params.Set("repo_name", repo)
	}

	path := "/api/backlog-items"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Data []BacklogItem `json:"data"`
	}
	if err := json.Unmarshal(resp, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return wrapper.Data, nil
}

func (c *Client) GetBacklogItem(id string) (*BacklogItem, error) {
	resp, err := c.doRequest("GET", "/api/backlog-items/"+id, nil)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Data BacklogItem `json:"data"`
	}
	if err := json.Unmarshal(resp, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &wrapper.Data, nil
}

func (c *Client) UpdateBacklogItem(id string, updates map[string]interface{}) (*BacklogItem, error) {
	body, err := json.Marshal(updates)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest("PUT", "/api/backlog-items/"+id, body)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Data BacklogItem `json:"data"`
	}
	if err := json.Unmarshal(resp, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &wrapper.Data, nil
}

func (c *Client) doRequest(method, path string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	} else if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	if c.workspaceID != "" {
		req.Header.Set("X-Workspace-ID", c.workspaceID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
