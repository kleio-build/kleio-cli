package main

import (
	"encoding/json"
	"testing"

	"github.com/kleio-build/kleio-cli/internal/cursorimport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildWatchCaptureInput_UsesCliSourceType(t *testing.T) {
	// Arrange
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "Detected a new work item",
	}

	// Act
	input := buildWatchCaptureInput(sig)

	// Assert
	assert.Equal(t, "cli", input.SourceType)
}

func TestBuildWatchCaptureInput_IncludesSignalHash(t *testing.T) {
	// Arrange
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "New task detected",
		SourceFile: "/home/user/.cursor/projects/slug/agent-transcripts/live.jsonl",
	}

	// Act
	input := buildWatchCaptureInput(sig)
	var sd map[string]interface{}
	require.NotNil(t, input.StructuredData)
	require.NoError(t, json.Unmarshal([]byte(*input.StructuredData), &sd))

	// Assert
	assert.Equal(t, "cursor_watch", sd["ingest_source"])
	assert.Equal(t, sig.Hash(), sd["signal_hash"])
	assert.Equal(t, sig.SourceFile, sd["file"])
}

func TestBuildWatchCaptureInput_FreeformContextContainsProvenance(t *testing.T) {
	// Arrange
	sig := cursorimport.Signal{
		SignalType: "decision",
		Content:    "Chose Redis for caching",
		SourceFile: "/path/to/session.jsonl",
	}

	// Act
	input := buildWatchCaptureInput(sig)

	// Assert
	require.NotNil(t, input.FreeformContext)
	assert.Contains(t, *input.FreeformContext, "Observed from Cursor agent transcript (live watch)")
	assert.Contains(t, *input.FreeformContext, "session.jsonl")
}

func TestBuildWatchCaptureInput_SameStructuredDataShapeAsCursorImport(t *testing.T) {
	// Arrange
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "Task from watch",
		SourceFile: "/path/to/file.jsonl",
	}

	// Act
	watchInput := buildWatchCaptureInput(sig)

	var watchSD map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(*watchInput.StructuredData), &watchSD))

	// Assert: same keys present
	assert.Contains(t, watchSD, "ingest_source")
	assert.Contains(t, watchSD, "signal_hash")
	assert.Contains(t, watchSD, "file")
	assert.Equal(t, sig.Hash(), watchSD["signal_hash"])
}
