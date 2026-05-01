package commands

import (
	"encoding/json"
	"path/filepath"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/cursorimport"
)

// Legacy helpers still used by init.go and import_cursor_test.go.
// New code should not depend on these — use the pipeline instead.

func filterNewSignals(signals []cursorimport.Signal) []cursorimport.Signal {
	var result []cursorimport.Signal
	for _, s := range signals {
		if !s.AlreadyCaptured {
			result = append(result, s)
		}
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func buildCursorEvent(sig cursorimport.Signal, redactedContent string, origin cursorimport.ScopedTranscript) *kleio.Event {
	sd := map[string]interface{}{
		"ingest_source": "cursor_transcript",
		"signal_hash":   sig.Hash(),
	}
	if sig.SourceFile != "" {
		sd["file"] = sig.SourceFile
	}
	if origin.ProjectSlug != "" {
		sd["cursor_project"] = origin.ProjectSlug
	}
	if origin.RepoOwner != "" {
		sd["repo_owner"] = origin.RepoOwner
	}
	sdJSON, _ := json.Marshal(sd)

	provenance := "Imported from Cursor agent transcript"
	if sig.SourceFile != "" {
		provenance += " (" + filepath.Base(sig.SourceFile) + ")"
	}

	return &kleio.Event{
		Content:         redactedContent,
		SignalType:      sig.SignalType,
		SourceType:      kleio.SourceTypeCursorTranscript,
		RepoName:        origin.RepoName,
		StructuredData:  string(sdJSON),
		FreeformContext: provenance,
	}
}
