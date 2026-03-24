package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
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

	if err := HealthCheck(srv.URL, srv.Client()); err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if out.CapturesCount != 3 || out.BacklogItemsCount != 2 {
		t.Fatalf("counts: %+v", out)
	}
	if out.BacklogByStatus["new"] != 2 {
		t.Fatalf("by status: %+v", out.BacklogByStatus)
	}
}
