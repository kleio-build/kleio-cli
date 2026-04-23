package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	require.NoError(t, HealthCheck(srv.URL, srv.Client()))
}

func TestGetWorkspaceCounts_parse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspace/counts" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-API-Key") != "k" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Workspace-ID") != "ws1" {
			http.Error(w, "workspace", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"workspace_id":"ws1","workspace_name":"W","workspace_slug":"w","captures_count":3,"backlog_items_count":2,"backlog_by_status":{"new":2}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "ws1")
	out, err := c.GetWorkspaceCounts()
	require.NoError(t, err)
	require.Equal(t, int64(3), out.CapturesCount)
	require.Equal(t, int64(2), out.BacklogItemsCount)
	require.Equal(t, int64(2), out.BacklogByStatus["new"])
}

// TestGetWebhookHealth_parse pins the wire shape so a server-side rename
// (e.g. failures_24h -> dlq_count_24h) trips the CLI test before the user
// sees a silently-empty status line.
func TestGetWebhookHealth_parse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health/webhooks" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"failures_24h":3,"oldest_unresolved_at":"2026-04-22T10:00:00Z","last_failure_at":"2026-04-22T11:30:00Z"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "ws1")
	out, err := c.GetWebhookHealth()
	require.NoError(t, err)
	require.Equal(t, int64(3), out.Failures24h)
	require.NotNil(t, out.OldestUnresolvedAt)
	require.Equal(t, "2026-04-22T10:00:00Z", *out.OldestUnresolvedAt)
}

// TestGetWebhookHealth_cleanDB pins the omitted-fields contract: pointer
// fields must round-trip as nil so the status command's `if != nil` print
// guard works.
func TestGetWebhookHealth_cleanDB(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"failures_24h":0}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "ws1")
	out, err := c.GetWebhookHealth()
	require.NoError(t, err)
	require.Equal(t, int64(0), out.Failures24h)
	require.Nil(t, out.OldestUnresolvedAt)
	require.Nil(t, out.LastFailureAt)
}
