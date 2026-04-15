package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kleio-build/kleio-cli/internal/config"
)

// ErrAuthRequired is returned when authentication is needed but not configured.
var ErrAuthRequired = errors.New("authentication required: run `kleio login` to authenticate; the MCP server reloads ~/.kleio/config.yaml periodically, or restart MCP/Cursor to pick up tokens immediately")

const defaultHTTPTimeout = 90 * time.Second

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

	refreshMu      sync.RWMutex
	onTokenRefresh OnTokenRefreshFunc
}

// New creates a client that uses X-API-Key auth. workspaceID is sent on every request
// as X-Workspace-ID when non-empty (required for most /api routes).
func New(baseURL, apiKey, workspaceID string) *Client {
	return &Client{
		baseURL:     baseURL,
		apiKey:      apiKey,
		workspaceID: workspaceID,
		httpClient:  &http.Client{Timeout: defaultHTTPTimeout},
	}
}

func NewWithToken(baseURL, token, workspaceID string) *Client {
	return &Client{
		baseURL:     baseURL,
		token:       token,
		workspaceID: workspaceID,
		httpClient:  &http.Client{Timeout: defaultHTTPTimeout},
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
		httpClient:   &http.Client{Timeout: defaultHTTPTimeout},
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
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	c.workspaceID = id
}

// WorkspaceID returns the currently configured workspace ID.
func (c *Client) WorkspaceID() string {
	c.refreshMu.RLock()
	defer c.refreshMu.RUnlock()
	return c.workspaceID
}

// BaseURL returns the API base URL (for tests and diagnostics).
func (c *Client) BaseURL() string {
	c.refreshMu.RLock()
	defer c.refreshMu.RUnlock()
	return c.baseURL
}

// ReloadFromConfig replaces in-memory credentials and base URL from a freshly loaded config file.
// Returns true if any auth-related field or base URL changed. Workspace is only overwritten when
// cfg.WorkspaceID is non-empty.
func (c *Client) ReloadFromConfig(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	prevTok, prevRef, prevKey, prevWS, prevBase := c.token, c.refreshToken, c.apiKey, c.workspaceID, c.baseURL
	c.token = strings.TrimSpace(cfg.Token)
	c.refreshToken = strings.TrimSpace(cfg.RefreshToken)
	c.apiKey = strings.TrimSpace(cfg.APIKey)
	c.baseURL = strings.TrimSpace(cfg.APIURL)
	if c.baseURL == "" {
		c.baseURL = prevBase
	}
	if ws := strings.TrimSpace(cfg.WorkspaceID); ws != "" {
		c.workspaceID = ws
	}
	return c.token != prevTok || c.refreshToken != prevRef || c.apiKey != prevKey || c.workspaceID != prevWS || c.baseURL != prevBase
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
	TitleHint       *string  `json:"title_hint,omitempty"`
}

type CaptureResult struct {
	CaptureID         string  `json:"capture_id"`
	LinkedBacklogItem string  `json:"linked_backlog_item_id"`
	Action            string  `json:"action"`
	DedupeConfidence  float64 `json:"dedupe_confidence"`
}

type CaptureListItem struct {
	ID           string  `json:"id"`
	Content      string  `json:"content"`
	SignalType   string  `json:"signal_type"`
	AuthorType   string  `json:"author_type"`
	RepoName     *string `json:"repo_name"`
	CreatedAt    string  `json:"created_at"`
	WorkspaceID  string  `json:"workspace_id"`
}

type ListCapturesOptions struct {
	CreatedAfter string
	AuthorType   string
	SignalType   string
}

type BacklogItem struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Summary          string   `json:"summary"`
	Category         string   `json:"category"`
	Urgency          string   `json:"urgency"`
	Importance       string   `json:"importance"`
	Status           string   `json:"status"`
	RepoName         *string  `json:"repo_name"`
	AssigneeID       *string  `json:"assignee_id"`
	ShortID          *int     `json:"short_id"`
	DedupeConfidence *float64 `json:"dedupe_confidence"`
	WorkspaceID      string   `json:"workspace_id"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

// BacklogListFilters are applied client-side after GET /api/backlog-items.
type BacklogListFilters struct {
	Status, Urgency, Importance, Category, Repo string
	Search, Assignee                             string
	Limit                                        int
}

func filterBacklogItemsCLI(items []BacklogItem, f BacklogListFilters) []BacklogItem {
	search := strings.ToLower(strings.TrimSpace(f.Search))
	var out []BacklogItem
	for _, it := range items {
		if f.Status != "" && it.Status != f.Status {
			continue
		}
		if f.Urgency != "" && it.Urgency != f.Urgency {
			continue
		}
		if f.Importance != "" && it.Importance != f.Importance {
			continue
		}
		if f.Category != "" && it.Category != f.Category {
			continue
		}
		if f.Repo != "" && (it.RepoName == nil || *it.RepoName != f.Repo) {
			continue
		}
		if search != "" {
			hay := strings.ToLower(it.Title + " " + it.Summary)
			if !strings.Contains(hay, search) {
				continue
			}
		}
		if a := strings.TrimSpace(f.Assignee); a != "" && !strings.EqualFold(a, "self") {
			if it.AssigneeID == nil || *it.AssigneeID != a {
				continue
			}
		}
		out = append(out, it)
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out
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

func (c *Client) ListCaptures(opts ListCapturesOptions) ([]CaptureListItem, error) {
	params := url.Values{}
	if opts.CreatedAfter != "" {
		params.Set("created_after", opts.CreatedAfter)
	}
	if opts.AuthorType != "" {
		params.Set("author_type", opts.AuthorType)
	}
	if opts.SignalType != "" {
		params.Set("signal_type", opts.SignalType)
	}

	path := "/api/captures"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Data []CaptureListItem `json:"data"`
	}
	if err := json.Unmarshal(resp, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return wrapper.Data, nil
}

func (c *Client) ListBacklogItems(f BacklogListFilters) ([]BacklogItem, error) {
	params := url.Values{}
	if f.Status != "" {
		params.Set("status", f.Status)
	}
	if f.Urgency != "" {
		params.Set("urgency", f.Urgency)
	}
	if f.Importance != "" {
		params.Set("importance", f.Importance)
	}
	if f.Category != "" {
		params.Set("category", f.Category)
	}
	if f.Repo != "" {
		params.Set("repo_name", f.Repo)
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

	if strings.EqualFold(strings.TrimSpace(f.Assignee), "self") {
		me, merr := c.GetMeJSON()
		if merr == nil {
			if uid, _ := me["id"].(string); strings.TrimSpace(uid) != "" {
				f.Assignee = uid
			}
		}
	}

	return filterBacklogItemsCLI(wrapper.Data, f), nil
}

var backlogKLRefPattern = regexp.MustCompile(`(?i)^#?KL-(\d+)$`)

// ResolveBacklogItemRef maps a backlog UUID or KL-N reference to the item UUID (client-side scan).
func (c *Client) ResolveBacklogItemRef(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("id is required")
	}
	if _, err := uuid.Parse(raw); err == nil {
		return raw, nil
	}
	raw = strings.TrimPrefix(raw, "#")
	m := backlogKLRefPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(m) != 2 {
		return "", fmt.Errorf("invalid backlog id (use UUID or KL-<number>)")
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return "", fmt.Errorf("invalid backlog short id")
	}
	items, err := c.ListBacklogItems(BacklogListFilters{})
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.ShortID != nil && *it.ShortID == n {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("backlog item not found for %s", m[0])
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

// MemoryQueryResult is the response from the memory query API.
type MemoryQueryResult struct {
	Answer  string         `json:"answer"`
	Sources []MemorySource `json:"sources"`
}

type MemorySource struct {
	CaptureID  string  `json:"capture_id"`
	Content    string  `json:"content"`
	SignalType string  `json:"signal_type"`
	RepoName   string  `json:"repo_name,omitempty"`
	BranchName string  `json:"branch_name,omitempty"`
	FilePath   string  `json:"file_path,omitempty"`
	CreatedAt  string  `json:"created_at"`
	Similarity float64 `json:"similarity"`
}

func (c *Client) AskMemory(question string) (*MemoryQueryResult, error) {
	body, err := json.Marshal(map[string]string{"question": question})
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest("POST", "/api/memory/query", body)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Data MemoryQueryResult `json:"data"`
	}
	if err := json.Unmarshal(resp, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &wrapper.Data, nil
}

// ADRFileInput represents a single ADR file to import.
type ADRFileInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// ADRImportResult summarizes the result of an ADR import.
type ADRImportResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
	Items    []struct {
		FilePath  string `json:"file_path"`
		Title     string `json:"title"`
		Format    string `json:"format"`
		CaptureID string `json:"capture_id,omitempty"`
		Skipped   bool   `json:"skipped"`
		Reason    string `json:"reason,omitempty"`
	} `json:"items"`
}

func (c *Client) ImportADRs(repoName string, files []ADRFileInput) (*ADRImportResult, error) {
	payload := map[string]interface{}{
		"repo_name": repoName,
		"files":     files,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest("POST", "/api/import/adr", body)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Data ADRImportResult `json:"data"`
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

	c.refreshMu.RLock()
	hasRefresh := c.refreshToken != ""
	c.refreshMu.RUnlock()
	if statusCode == http.StatusUnauthorized && hasRefresh {
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

	c.refreshMu.RLock()
	baseURL := c.baseURL
	token := c.token
	apiKey := c.apiKey
	workspaceID := c.workspaceID
	c.refreshMu.RUnlock()

	req, err := http.NewRequest(method, baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	if workspaceID != "" {
		req.Header.Set("X-Workspace-ID", workspaceID)
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
