package commands

import (
	"encoding/json"
	"testing"

	"github.com/kleio-build/kleio-cli/internal/cursorimport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCursorEvent_UsesCliSourceType(t *testing.T) {
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "Implement auth flow",
	}
	evt := buildCursorEvent(sig, sig.Content)
	assert.Equal(t, "cli", evt.SourceType)
}

func TestBuildCursorEvent_IncludesSignalHash(t *testing.T) {
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "Implement auth flow",
		SourceFile: "/home/user/.cursor/projects/slug/agent-transcripts/abc.jsonl",
	}

	evt := buildCursorEvent(sig, sig.Content)
	var sd map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(evt.StructuredData), &sd))

	assert.Equal(t, "cursor_transcript", sd["ingest_source"])
	assert.Equal(t, sig.Hash(), sd["signal_hash"])
	assert.Equal(t, sig.SourceFile, sd["file"])
}

func TestBuildCursorEvent_FreeformContextContainsProvenance(t *testing.T) {
	sig := cursorimport.Signal{
		SignalType: "decision",
		Content:    "Use JWT for auth",
		SourceFile: "/path/to/transcript.jsonl",
	}

	evt := buildCursorEvent(sig, sig.Content)
	assert.Contains(t, evt.FreeformContext, "Imported from Cursor agent transcript")
	assert.Contains(t, evt.FreeformContext, "transcript.jsonl")
}

func TestBuildCursorEvent_NoSourceFile(t *testing.T) {
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "Fix a bug",
	}

	evt := buildCursorEvent(sig, sig.Content)
	var sd map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(evt.StructuredData), &sd))

	assert.Nil(t, sd["file"], "file should not be present when SourceFile is empty")
	assert.NotContains(t, evt.FreeformContext, "()")
}

func TestBuildCursorEvent_UsesRedactedContent(t *testing.T) {
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "Set API key to sk-secret123",
	}
	redacted := "Set API key to [REDACTED]"

	evt := buildCursorEvent(sig, redacted)
	assert.Equal(t, redacted, evt.Content, "should use redacted content, not original")
}
