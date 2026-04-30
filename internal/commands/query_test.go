package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/stretchr/testify/require"
)

func testStore(t *testing.T) kleio.Store {
	t.Helper()
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return localdb.New(db)
}

func TestQueryCaptures_HumanOutput(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID: "abc12345-aaaa-bbbb-cccc-1234567890ab", SignalType: "decision",
		Content: "first capture", SourceType: "manual", RepoName: "kleio-app",
	}))
	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID: "def67890-aaaa-bbbb-cccc-1234567890ab", SignalType: "checkpoint",
		Content: "second capture", SourceType: "manual",
	}))

	cmd := NewQueryCmd(func() kleio.Store { return store })
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
	require.Contains(t, out, "2 events")
}

func TestQueryCaptures_JSONOutput(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		SignalType: "decision", Content: "x", SourceType: "manual",
	}))

	cmd := NewQueryCmd(func() kleio.Store { return store })
	cmd.SetArgs([]string{"captures", "--json"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, cmd.Execute())

	var got []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 1)
}

func TestQuerySemantic_RequiresQuery(t *testing.T) {
	store := testStore(t)
	cmd := NewQueryCmd(func() kleio.Store { return store })
	cmd.SetArgs([]string{"semantic"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	require.Error(t, err)
}

func TestQuerySemantic_HumanOutput(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		SignalType: "decision", Content: "how we picked bcrypt for auth",
		SourceType: "manual",
	}))

	cmd := NewQueryCmd(func() kleio.Store { return store })
	cmd.SetArgs([]string{"semantic", "bcrypt"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, cmd.Execute())

	out := buf.String()
	require.Contains(t, out, "bcrypt")
}

func TestQueryCapture_DetailJSON(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID: "abc-id", SignalType: "decision", Content: "detail",
		SourceType: "manual",
	}))

	cmd := NewQueryCmd(func() kleio.Store { return store })
	cmd.SetArgs([]string{"capture", "abc-id"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, cmd.Execute())

	out := buf.String()
	require.Contains(t, out, "abc-id")
	require.Contains(t, out, "detail")
}

func TestQueryBacklog_HumanOutput(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	require.NoError(t, store.CreateBacklogItem(ctx, &kleio.BacklogItem{
		Title: "Add JWT auth", Summary: "implement bcrypt + JWT TTL",
		Status: "in_progress", Category: "feature",
		Urgency: "high", Importance: "high",
	}))

	cmd := NewQueryCmd(func() kleio.Store { return store })
	cmd.SetArgs([]string{"backlog", "--status", "in_progress"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, cmd.Execute())

	out := buf.String()
	require.Contains(t, out, "Add JWT auth")
	require.Contains(t, out, "in_progress")
	require.Contains(t, out, "1 items")
}

func TestQueryBacklogShow_DetailJSON(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	item := &kleio.BacklogItem{
		ID: "abc-id", Title: "x", Summary: "y", Status: "open",
	}
	require.NoError(t, store.CreateBacklogItem(ctx, item))

	cmd := NewQueryCmd(func() kleio.Store { return store })
	cmd.SetArgs([]string{"backlog-show", "abc-id"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, cmd.Execute())

	out := buf.String()
	require.Contains(t, out, "abc-id")
	require.Contains(t, out, "\"title\"")
}

func TestOneLine_TruncatesLongContent(t *testing.T) {
	long := strings.Repeat("a", 200)
	result := oneLine(long, 100)
	require.True(t, strings.Contains(result, "…"), "long content should be ellipsized")
	require.Less(t, len(result), 200, "truncated result should be shorter than input")
}

func TestShortID(t *testing.T) {
	require.Equal(t, "abcdefgh", shortID("abcdefgh-aaaa-bbbb-cccc-1234567890ab"))
	require.Equal(t, "abc", shortID("abc"))
}
