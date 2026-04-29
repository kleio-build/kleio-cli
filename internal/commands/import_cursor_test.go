package commands

import (
	"encoding/json"
	"testing"

	"github.com/kleio-build/kleio-cli/internal/cursorimport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCursorCaptureInput_UsesCliSourceType(t *testing.T) {
	// Arrange
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "Implement auth flow",
	}

	// Act
	input := buildCursorCaptureInput(sig, sig.Content)

	// Assert
	assert.Equal(t, "cli", input.SourceType)
}

func TestBuildCursorCaptureInput_IncludesSignalHash(t *testing.T) {
	// Arrange
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "Implement auth flow",
		SourceFile: "/home/user/.cursor/projects/slug/agent-transcripts/abc.jsonl",
	}

	// Act
	input := buildCursorCaptureInput(sig, sig.Content)
	var sd map[string]interface{}
	require.NotNil(t, input.StructuredData)
	require.NoError(t, json.Unmarshal([]byte(*input.StructuredData), &sd))

	// Assert
	assert.Equal(t, "cursor_transcript", sd["ingest_source"])
	assert.Equal(t, sig.Hash(), sd["signal_hash"])
	assert.Equal(t, sig.SourceFile, sd["file"])
}

func TestBuildCursorCaptureInput_FreeformContextContainsProvenance(t *testing.T) {
	// Arrange
	sig := cursorimport.Signal{
		SignalType: "decision",
		Content:    "Use JWT for auth",
		SourceFile: "/path/to/transcript.jsonl",
	}

	// Act
	input := buildCursorCaptureInput(sig, sig.Content)

	// Assert
	require.NotNil(t, input.FreeformContext)
	assert.Contains(t, *input.FreeformContext, "Imported from Cursor agent transcript")
	assert.Contains(t, *input.FreeformContext, "transcript.jsonl")
}

func TestBuildCursorCaptureInput_NoSourceFile(t *testing.T) {
	// Arrange
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "Fix a bug",
	}

	// Act
	input := buildCursorCaptureInput(sig, sig.Content)
	var sd map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(*input.StructuredData), &sd))

	// Assert
	assert.Nil(t, sd["file"], "file should not be present when SourceFile is empty")
	assert.NotContains(t, *input.FreeformContext, "()")
}

func TestBuildCursorCaptureInput_UsesRedactedContent(t *testing.T) {
	// Arrange
	sig := cursorimport.Signal{
		SignalType: "work_item",
		Content:    "Set API key to sk-secret123",
	}
	redacted := "Set API key to [REDACTED]"

	// Act
	input := buildCursorCaptureInput(sig, redacted)

	// Assert
	assert.Equal(t, redacted, input.Content, "should use redacted content, not original")
}
