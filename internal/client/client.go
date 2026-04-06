package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// ErrAuthRequired is returned when authentication is needed but not configured.
var ErrAuthRequired = errors.New("authentication required: run `kleio login` to authenticate, then restart the MCP server")

// OnTokenRefreshFunc is called after a successful token refresh so callers
// (e.g. the MCP server) can persist the new tokens.
type OnTokenRefreshFunc func(newToken, newRefreshToken string)

type Client struct {
	baseURL      string
	apiKey       string
	token        string
	refreshToken string
	workspaceID  string
	httpClient   *http.Client

	refreshMu      sync.Mutex
	onTokenRefresh OnTokenRefreshFunc
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

// NewWithTokens creates a client with both access and refresh tokens, enabling
// automatic token refresh on 401 responses.
func NewWithTokens(baseURL, token, refreshToken, workspaceID string) *Client {
	return &Client{
		baseURL:      baseURL,
		token:        token,
		refreshToken: refreshToken,
		workspaceID:  workspaceID,
		httpClient:   &http.Client{},
	}
}

// SetOnTokenRefresh registers a callback invoked after a successful token
// refresh. Use this to persist new tokens to disk.
func (c *Client) SetOnTokenRefresh(fn OnTokenRefreshFunc) {
	c.onTokenRefresh = fn
}

// SetWorkspaceID overrides the workspace after construction. Used by the
// workspace resolution chain when the workspace is determined dynamically
// (e.g. from git remote or project config).
func (c *Client) SetWorkspaceID(id string) {
	c.workspaceID = id
}

// WorkspaceID returns the currently configured workspace ID.
func (c *Client) WorkspaceID() string {
	return c.workspaceID
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

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// DeviceTokenResult wraps the API response for device token polling. Pending is
// true while the user hasn't authorized yet; check Error for terminal failures.
type DeviceTokenResult struct {
	TokenResponse
	Pending  bool   `json:"-"`
	Error    string `json:"error,omitempty"`
	Interval int    `json:"interval,omitempty"`
}

func (c *Client) RequestDeviceCode() (*DeviceCodeResponse, error) {
	resp, err := c.doRequestRaw("POST", "/auth/device/code", nil)
	if err != nil {
		return nil, err
	}
	var result DeviceCodeResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse device code response: %w", err)
	}
	return &result, nil
}

// PollDeviceToken polls the API for device flow completion. Returns a result
// with Pending=true if the user hasn't authorized yet.
func (c *Client) PollDeviceToken(deviceCode string) (*DeviceTokenResult, error) {
	body, _ := json.Marshal(map[string]string{"device_code": deviceCode})

	req, err := http.NewRequest("POST", c.baseURL+"/auth/device/token", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusAccepted {
		var result DeviceTokenResult
		json.Unmarshal(respBody, &result)
		result.Pending = true
		return &result, nil
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		json.Unmarshal(respBody, &errResp)
		return &DeviceTokenResult{Error: errResp.Error}, nil
	}

	var result DeviceTokenResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	return &result, nil
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

// doRequestRaw performs an HTTP request without auth headers or auto-refresh.
func (c *Client) doRequestRaw(method, path string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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

func (c *Client) doRequest(method, path string, body []byte) ([]byte, error) {
	respBody, statusCode, err := c.doRequestOnce(method, path, body)
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusUnauthorized && c.refreshToken != "" {
		if refreshErr := c.tryRefresh(); refreshErr == nil {
			respBody, statusCode, err = c.doRequestOnce(method, path, body)
			if err != nil {
				return nil, err
			}
		}
	}

	if statusCode == http.StatusUnauthorized {
		return nil, ErrAuthRequired
	}

	if statusCode >= 400 {
		return nil, fmt.Errorf("API error (%d): %s", statusCode, string(respBody))
	}

	return respBody, nil
}

func (c *Client) doRequestOnce(method, path string, body []byte) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
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
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

func (c *Client) tryRefresh() error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	if c.refreshToken == "" {
		return fmt.Errorf("no refresh token")
	}

	body, _ := json.Marshal(map[string]string{"refresh_token": c.refreshToken})
	resp, err := c.doRequestRaw("POST", "/auth/refresh", body)
	if err != nil {
		return err
	}

	var tokens TokenResponse
	if err := json.Unmarshal(resp, &tokens); err != nil {
		return err
	}

	c.token = tokens.AccessToken
	c.refreshToken = tokens.RefreshToken

	if c.onTokenRefresh != nil {
		c.onTokenRefresh(tokens.AccessToken, tokens.RefreshToken)
	}

	return nil
}
