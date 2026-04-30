package main

import (
	"encoding/json"
	"testing"

	"github.com/kleio-build/kleio-cli/internal/cursorimport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildWatchEvent_UsesCliSourceType(t *testing.T) {
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "Detected a new work item",
	}
	evt := buildWatchEvent(sig)
	assert.Equal(t, "cli", evt.SourceType)
}

func TestBuildWatchEvent_IncludesSignalHash(t *testing.T) {
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "New task detected",
		SourceFile: "/home/user/.cursor/projects/slug/agent-transcripts/live.jsonl",
	}

	evt := buildWatchEvent(sig)
	var sd map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(evt.StructuredData), &sd))

	assert.Equal(t, "cursor_watch", sd["ingest_source"])
	assert.Equal(t, sig.Hash(), sd["signal_hash"])
	assert.Equal(t, sig.SourceFile, sd["file"])
}

func TestBuildWatchEvent_FreeformContextContainsProvenance(t *testing.T) {
	sig := cursorimport.Signal{
		SignalType: "decision",
		Content:    "Chose Redis for caching",
		SourceFile: "/path/to/session.jsonl",
	}

	evt := buildWatchEvent(sig)
	assert.Contains(t, evt.FreeformContext, "Observed from Cursor agent transcript (live watch)")
	assert.Contains(t, evt.FreeformContext, "session.jsonl")
}

func TestBuildWatchEvent_SameStructuredDataShapeAsCursorImport(t *testing.T) {
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "Task from watch",
		SourceFile: "/path/to/file.jsonl",
	}

	evt := buildWatchEvent(sig)
	var watchSD map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(evt.StructuredData), &watchSD))

	assert.Contains(t, watchSD, "ingest_source")
	assert.Contains(t, watchSD, "signal_hash")
	assert.Contains(t, watchSD, "file")
	assert.Equal(t, sig.Hash(), watchSD["signal_hash"])
}
