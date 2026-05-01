package cursorimport

// Coverage contract:
//
// TranscriptParser (explicit signals only — implicit heuristics removed):
// - extracts explicit kleio_decide tool calls as decision signals
// - extracts explicit kleio_capture tool calls as work_item signals
// - extracts explicit kleio_checkpoint tool calls as checkpoint signals (for dedup, not re-ingested)
// - marks already-captured signals with AlreadyCaptured=true
// - extracts file provenance from Write/StrReplace/Read tool calls
// - handles malformed JSON lines gracefully (skip, don't crash)
// - handles empty input
// - extracts session metadata (role counts, tool call counts)
// - deduplicates signals within same transcript
//
// ParseFile:
// - reads a real JSONL file and returns signals
//
// Signal hashing:
// - same content at same line produces same hash
// - different content produces different hash

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_ExtractsDecisionFromToolCall(t *testing.T) {
	// Arrange
	lines := []string{
		`{"role":"assistant","message":{"content":[{"type":"text","text":"I'll use Option B because..."},{"type":"tool_use","name":"kleio_decide","input":{"content":"Use PostgreSQL for new services","rationale":"Better JSON support and ecosystem"}}]}}`,
	}
	parser := NewTranscriptParser()

	// Act
	result, err := parser.ParseLines(lines)

	// Assert
	require.NoError(t, err)
	require.Len(t, result.Signals, 1)
	assert.Equal(t, "decision", result.Signals[0].SignalType)
	assert.Contains(t, result.Signals[0].Content, "Use PostgreSQL")
	assert.True(t, result.Signals[0].AlreadyCaptured)
}

func TestParser_ExtractsCaptureToolCall(t *testing.T) {
	// Arrange
	lines := []string{
		`{"role":"assistant","message":{"content":[{"type":"tool_use","name":"kleio_capture","input":{"content":"Need to add rate limiting to the API","signal_type":"work_item"}}]}}`,
	}
	parser := NewTranscriptParser()

	// Act
	result, err := parser.ParseLines(lines)

	// Assert
	require.NoError(t, err)
	require.Len(t, result.Signals, 1)
	assert.Equal(t, "work_item", result.Signals[0].SignalType)
	assert.Contains(t, result.Signals[0].Content, "rate limiting")
	assert.True(t, result.Signals[0].AlreadyCaptured)
}

func TestParser_ExtractsCheckpointToolCall(t *testing.T) {
	// Arrange
	lines := []string{
		`{"role":"assistant","message":{"content":[{"type":"tool_use","name":"kleio_checkpoint","input":{"content":"Completed auth module with OAuth2","slice_category":"feature","slice_status":"complete","validation_status":"tests_pass"}}]}}`,
	}
	parser := NewTranscriptParser()

	// Act
	result, err := parser.ParseLines(lines)

	// Assert
	require.NoError(t, err)
	require.Len(t, result.Signals, 1)
	assert.Equal(t, "checkpoint", result.Signals[0].SignalType)
	assert.Contains(t, result.Signals[0].Content, "auth module")
	assert.True(t, result.Signals[0].AlreadyCaptured)
}

func TestParser_ExtractsFileProvenance(t *testing.T) {
	// Arrange
	lines := []string{
		`{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"path":"internal/auth/handler.go","contents":"package auth"}},{"type":"tool_use","name":"Read","input":{"path":"go.mod"}},{"type":"tool_use","name":"StrReplace","input":{"path":"internal/config/config.go","old_string":"old","new_string":"new"}}]}}`,
	}
	parser := NewTranscriptParser()

	// Act
	result, err := parser.ParseLines(lines)

	// Assert
	require.NoError(t, err)
	assert.Len(t, result.FilesTouched, 3)
	assert.Contains(t, result.FilesTouched, "internal/auth/handler.go")
	assert.Contains(t, result.FilesTouched, "go.mod")
	assert.Contains(t, result.FilesTouched, "internal/config/config.go")
}

func TestParser_SkipsMalformedJSON(t *testing.T) {
	// Arrange
	lines := []string{
		`not valid json`,
		`{"role":"assistant","message":{"content":[{"type":"tool_use","name":"kleio_capture","input":{"content":"valid signal","signal_type":"work_item"}}]}}`,
		`{"truncated":`,
	}
	parser := NewTranscriptParser()

	// Act
	result, err := parser.ParseLines(lines)

	// Assert
	require.NoError(t, err)
	assert.Len(t, result.Signals, 1)
}

func TestParser_EmptyInput(t *testing.T) {
	// Arrange
	parser := NewTranscriptParser()

	// Act
	result, err := parser.ParseLines(nil)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, result.Signals)
}

func TestParser_SessionMetadata(t *testing.T) {
	// Arrange
	lines := []string{
		`{"role":"user","message":{"content":[{"type":"text","text":"Do something"}]}}`,
		`{"role":"assistant","message":{"content":[{"type":"text","text":"Done"},{"type":"tool_use","name":"Write","input":{"path":"foo.go","contents":"pkg"}}]}}`,
		`{"role":"user","message":{"content":[{"type":"text","text":"Thanks"}]}}`,
	}
	parser := NewTranscriptParser()

	// Act
	result, err := parser.ParseLines(lines)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 2, result.UserTurns)
	assert.Equal(t, 1, result.AssistantTurns)
	assert.Equal(t, 1, result.ToolCallCount)
}

func TestParser_SignalHash(t *testing.T) {
	// Arrange
	s1 := Signal{Content: "Use PostgreSQL", SignalType: "decision", LineOffset: 5}
	s2 := Signal{Content: "Use PostgreSQL", SignalType: "decision", LineOffset: 10}
	s3 := Signal{Content: "Use MySQL", SignalType: "decision", LineOffset: 5}

	// Act & Assert — same content produces same hash regardless of line offset
	assert.Equal(t, s1.Hash(), s2.Hash())
	assert.NotEqual(t, s1.Hash(), s3.Hash())
}

func TestParser_DedupesWithinTranscript(t *testing.T) {
	// Arrange
	lines := []string{
		`{"role":"assistant","message":{"content":[{"type":"tool_use","name":"kleio_capture","input":{"content":"Add rate limiting","signal_type":"work_item"}}]}}`,
		`{"role":"assistant","message":{"content":[{"type":"tool_use","name":"kleio_capture","input":{"content":"Add rate limiting","signal_type":"work_item"}}]}}`,
	}
	parser := NewTranscriptParser()

	// Act
	result, err := parser.ParseLines(lines)

	// Assert
	require.NoError(t, err)
	assert.Len(t, result.Signals, 1)
}

func TestParser_NoImplicitSignalWhenKleioCallPresent(t *testing.T) {
	// Line has both: assistant text with decision language AND an explicit kleio_decide call.
	// Only the explicit call should produce a signal.
	lines := []string{
		`{"role":"assistant","message":{"content":[{"type":"text","text":"I'll go with PostgreSQL because it has better JSON support."},{"type":"tool_use","name":"kleio_decide","input":{"content":"Use PostgreSQL","rationale":"Better JSON support"}}]}}`,
	}
	parser := NewTranscriptParser()

	result, err := parser.ParseLines(lines)

	require.NoError(t, err)
	require.Len(t, result.Signals, 1)
	assert.Equal(t, "decision", result.Signals[0].SignalType)
	assert.True(t, result.Signals[0].AlreadyCaptured)
}

func TestParser_ShortTextIgnored(t *testing.T) {
	lines := []string{
		`{"role":"assistant","message":{"content":[{"type":"text","text":"Done."}]}}`,
	}
	parser := NewTranscriptParser()

	result, err := parser.ParseLines(lines)

	require.NoError(t, err)
	assert.Empty(t, result.Signals)
}

func TestParser_CodeSentencesFiltered(t *testing.T) {
	lines := []string{
		`{"role":"assistant","message":{"content":[{"type":"text","text":"I'll go with this approach. Here is the implementation: func main() { fmt.Println('hello') } and then we call the function to start the application process."}]}}`,
	}
	parser := NewTranscriptParser()

	result, err := parser.ParseLines(lines)

	require.NoError(t, err)
	for _, s := range result.Signals {
		assert.NotContains(t, s.Content, "func main()",
			"code sentences should be filtered out")
	}
}

func TestParser_InsteadOfNotADecision(t *testing.T) {
	lines := []string{
		`{"role":"assistant","message":{"content":[{"type":"text","text":"I updated the function to return an error instead of panicking. This approach is more idiomatic in Go and allows callers to handle the failure gracefully."}]}}`,
	}
	parser := NewTranscriptParser()

	result, err := parser.ParseLines(lines)

	require.NoError(t, err)
	for _, s := range result.Signals {
		assert.NotEqual(t, "decision", s.SignalType,
			"'instead of' in middle of prose should not trigger decision")
	}
}

func TestParseFile_ReadsJSONL(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"role":"user","message":{"content":[{"type":"text","text":"Hello"}]}}
{"role":"assistant","message":{"content":[{"type":"tool_use","name":"kleio_capture","input":{"content":"Test signal","signal_type":"work_item"}}]}}
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	parser := NewTranscriptParser()

	// Act
	result, err := parser.ParseFile(path)

	// Assert
	require.NoError(t, err)
	assert.Len(t, result.Signals, 1)
	assert.Equal(t, path, result.FilePath)
}
