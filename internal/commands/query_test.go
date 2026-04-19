package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/stretchr/testify/require"
)

// stubCaptureServer returns a httptest server that replies to GET
// /api/memory/* with canned JSON, so we can exercise the Cobra commands and
// their human/json output without touching the real backend.
func stubMemoryServer(t *testing.T, payloadByPath map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, ok := payloadByPath[r.URL.Path]; ok {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func clientForServer(srv *httptest.Server) *client.Client {
	return client.New(srv.URL, "test-key", "ws-uuid")
}

func TestQueryCaptures_HumanOutput(t *testing.T) {
	srv := stubMemoryServer(t, map[string]string{
		"/api/memory/captures": `{"captures":[
			{"capture_id":"abc12345-aaaa-bbbb-cccc-1234567890ab","content":"first capture","signal_type":"decision","repo_name":"kleio-app","created_at":"2026-04-01T12:00:00Z"},
			{"capture_id":"def67890-aaaa-bbbb-cccc-1234567890ab","content":"second capture","signal_type":"checkpoint","created_at":"2026-04-01T13:00:00Z"}
		]}`,
	})
	cmd := NewQueryCmd(func() *client.Client { return clientForServer(srv) })
	cmd.SetArgs([]string{"captures"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, cmd.Execute())

	out := buf.String()
	require.Contains(t, out, "abc12345")
	require.Contains(t, out, "[decision]")
	require.Contains(t, out, "[kleio-app]")
	require.Contains(t, out, "first capture")
	require.Contains(t, out, "def67890")
	require.Contains(t, out, "[checkpoint]")
	require.Contains(t, out, "2 captures")
}

func TestQueryCaptures_JSONOutput(t *testing.T) {
	srv := stubMemoryServer(t, map[string]string{
		"/api/memory/captures": `{"captures":[{"capture_id":"abc12345","content":"x","signal_type":"decision","created_at":"2026-04-01T12:00:00Z"}]}`,
	})
	cmd := NewQueryCmd(func() *client.Client { return clientForServer(srv) })
	cmd.SetArgs([]string{"captures", "--json"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, cmd.Execute())

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Contains(t, got, "captures")
}

func TestQuerySemantic_RequiresQuery(t *testing.T) {
	srv := stubMemoryServer(t, map[string]string{})
	cmd := NewQueryCmd(func() *client.Client { return clientForServer(srv) })
	cmd.SetArgs([]string{"semantic"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	require.Error(t, err)
}

func TestQuerySemantic_HumanOutput(t *testing.T) {
	srv := stubMemoryServer(t, map[string]string{
		"/api/memory/semantic": `{"captures":[{"capture_id":"abc12345","content":"how we picked bcrypt","signal_type":"decision","created_at":"2026-04-01T12:00:00Z","similarity":0.87}]}`,
	})
	cmd := NewQueryCmd(func() *client.Client { return clientForServer(srv) })
	cmd.SetArgs([]string{"semantic", "auth library choice"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, cmd.Execute())

	out := buf.String()
	require.Contains(t, out, "0.87")
	require.Contains(t, out, "bcrypt")
}

func TestQueryCapture_DetailJSON(t *testing.T) {
	srv := stubMemoryServer(t, map[string]string{
		"/api/memory/captures/abc-id": `{"capture_id":"abc-id","content":"detail","signal_type":"decision","created_at":"2026-04-01T12:00:00Z","topics":["auth","jwt"]}`,
	})
	cmd := NewQueryCmd(func() *client.Client { return clientForServer(srv) })
	cmd.SetArgs([]string{"capture", "abc-id"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, cmd.Execute())

	out := buf.String()
	require.Contains(t, out, "abc-id")
	require.Contains(t, out, "auth")
	require.Contains(t, out, "jwt")
}

func TestQueryBacklog_HumanOutput(t *testing.T) {
	short := 42
	body, _ := json.Marshal(map[string]any{
		"backlog_items": []map[string]any{
			{
				"id":         "abc12345-aaaa-bbbb-cccc-1234567890ab",
				"short_id":   short,
				"title":      "Add JWT auth",
				"summary":    "implement bcrypt + JWT TTL",
				"status":     "in_progress",
				"category":   "feature",
				"urgency":    "high",
				"importance": "high",
				"created_at": "2026-04-01T12:00:00Z",
				"updated_at": "2026-04-01T12:00:00Z",
			},
		},
		"total_returned": 1,
	})
	srv := stubMemoryServer(t, map[string]string{
		"/api/memory/backlog": string(body),
	})
	cmd := NewQueryCmd(func() *client.Client { return clientForServer(srv) })
	cmd.SetArgs([]string{"backlog", "--status", "in_progress"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, cmd.Execute())

	out := buf.String()
	require.Contains(t, out, "KL-42")
	require.Contains(t, out, "Add JWT auth")
	require.Contains(t, out, "in_progress")
	require.Contains(t, out, "1 items")
}

func TestQueryBacklogShow_DetailJSON(t *testing.T) {
	srv := stubMemoryServer(t, map[string]string{
		"/api/memory/backlog/KL-7": `{"id":"abc","short_id":7,"title":"x","summary":"y","status":"new","linked_captures":[{"capture_id":"c1","link_type":"resolves"}],"linked_capture_count":1}`,
	})
	cmd := NewQueryCmd(func() *client.Client { return clientForServer(srv) })
	cmd.SetArgs([]string{"backlog-show", "KL-7"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, cmd.Execute())

	out := buf.String()
	require.Contains(t, out, `"linked_captures"`)
	require.Contains(t, out, "c1")
}

func TestPrintCaptureRow_TruncatesLongContent(t *testing.T) {
	long := strings.Repeat("a", 200)
	var buf bytes.Buffer
	printCaptureRow(&buf, client.MemoryCaptureHit{
		CaptureID:  "abcdefgh-aaaa-bbbb-cccc-1234567890ab",
		SignalType: "decision",
		Content:    long,
	})
	out := buf.String()
	require.Contains(t, out, "abcdefgh")
	require.True(t, strings.Contains(out, "…"), "long content should be ellipsized")
}

func TestShortID(t *testing.T) {
	require.Equal(t, "abcdefgh", shortID("abcdefgh-aaaa-bbbb-cccc-1234567890ab"))
	require.Equal(t, "abc", shortID("abc"))
}
