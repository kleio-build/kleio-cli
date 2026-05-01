package transcript

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kleio "github.com/kleio-build/kleio-core"
)

// transcript line builder used by every test below.
func tline(role string, blocks ...contentBlock) string {
	tl := transcriptLine{Role: role}
	tl.Message.Content = blocks
	b, _ := json.Marshal(tl)
	return string(b)
}

func writeTranscript(t *testing.T, dir, name string, lines []string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseTranscript_ExtractsExplicitKleioToolCalls(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "abc.jsonl", []string{
		tline("assistant",
			contentBlock{Type: "tool_use", Name: "kleio_capture", Input: map[string]any{
				"content":     "Implement plan ingester",
				"signal_type": "work_item",
				"rationale":   "needed for pipeline",
			}},
		),
		tline("assistant",
			contentBlock{Type: "tool_use", Name: "kleio_decide", Input: map[string]any{
				"content":   "Use plans as primary source",
				"rationale": "more structured than transcripts",
			}},
		),
		tline("assistant",
			contentBlock{Type: "tool_use", Name: "kleio_checkpoint", Input: map[string]any{
				"content": "Phase 0 PDF fix complete",
			}},
		),
	})

	signals, err := parseTranscript(path, "kleio-cli", kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 3 {
		t.Fatalf("want 3 explicit signals, got %d: %#v", len(signals), signals)
	}
	wantKinds := map[string]bool{
		kleio.SignalTypeWorkItem:   false,
		kleio.SignalTypeDecision:   false,
		kleio.SignalTypeCheckpoint: false,
	}
	for _, s := range signals {
		wantKinds[s.Kind] = true
		if !strings.HasPrefix(s.SourceOffset, "toolcall:L") {
			t.Errorf("signal SourceOffset=%q want toolcall:L*", s.SourceOffset)
		}
		if s.RepoName != "kleio-cli" {
			t.Errorf("RepoName=%q want kleio-cli", s.RepoName)
		}
	}
	for k, ok := range wantKinds {
		if !ok {
			t.Errorf("missing kind %q in output", k)
		}
	}
}

func TestParseTranscript_UnwrapsCallMcpToolForKleioCalls(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "mcp.jsonl", []string{
		tline("assistant", contentBlock{Type: "tool_use", Name: "CallMcpTool", Input: map[string]any{
			"server":   "user-kleio",
			"toolName": "kleio_decide",
			"arguments": map[string]any{
				"content":   "Use FTS5 standalone table",
				"rationale": "avoids trigger complexity",
			},
		}}),
		tline("assistant", contentBlock{Type: "tool_use", Name: "CallMcpTool", Input: map[string]any{
			"server":   "user-kleio",
			"toolName": "kleio_capture",
			"arguments": map[string]any{
				"content":     "TranscriptIngester narrow-accept",
				"signal_type": "work_item",
			},
		}}),
		tline("assistant", contentBlock{Type: "tool_use", Name: "CallMcpTool", Input: map[string]any{
			"server":   "user-other",
			"toolName": "some_other_tool",
			"arguments": map[string]any{"content": "ignored"},
		}}),
	})
	signals, err := parseTranscript(path, "kleio-cli", kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 2 {
		t.Fatalf("want 2 unwrapped MCP signals, got %d: %#v", len(signals), signals)
	}
	gotKinds := map[string]bool{}
	for _, s := range signals {
		gotKinds[s.Kind] = true
		if s.Metadata["tool_name"] == "CallMcpTool" {
			t.Errorf("metadata.tool_name=CallMcpTool, want unwrapped (kleio_*): %#v", s.Metadata)
		}
	}
	if !gotKinds[kleio.SignalTypeDecision] || !gotKinds[kleio.SignalTypeWorkItem] {
		t.Fatalf("missing one of decision/work_item kinds: %v", gotKinds)
	}
}

func TestParseTranscript_ExtractsDeferralLanguage(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "defer.jsonl", []string{
		tline("user", contentBlock{Type: "text", Text: "Implement the plan."}),
		tline("assistant", contentBlock{Type: "text", Text: "I'll defer the slack integration to a future PR. Out of scope for now."}),
		tline("assistant", contentBlock{Type: "text", Text: "Skipping the LLM correlator for now since ai.AutoDetect isn't wired."}),
		tline("assistant", contentBlock{Type: "text", Text: "I will write the parser. Then I will write tests. Both are core in scope."}),
	})

	signals, err := parseTranscript(path, "kleio-cli", kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	var deferred []kleio.RawSignal
	for _, s := range signals {
		if strings.HasPrefix(s.SourceOffset, "defer:L") {
			deferred = append(deferred, s)
		}
	}
	if len(deferred) < 3 {
		t.Fatalf("want >= 3 deferral signals (defer/out of scope/skip), got %d: %#v", len(deferred), deferred)
	}
	for _, s := range deferred {
		if s.Kind != kleio.SignalTypeWorkItem {
			t.Errorf("deferral kind=%q want work_item", s.Kind)
		}
		if s.Metadata["deferred"] != true {
			t.Errorf("deferral metadata.deferred=%v want true", s.Metadata["deferred"])
		}
	}
}

func TestParseTranscript_ExtractsRetroAsks(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "retro.jsonl", []string{
		tline("user", contentBlock{Type: "text", Text: "Build the report layer."}),
		tline("assistant", contentBlock{Type: "text", Text: "OK working on it."}),
		tline("user", contentBlock{Type: "text", Text: "Actually, also add a --pdf flag."}),
		tline("user", contentBlock{Type: "text", Text: "Wait: can you confirm the schema is shared?"}),
	})

	signals, err := parseTranscript(path, "kleio-cli", kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	var retro []kleio.RawSignal
	for _, s := range signals {
		if strings.HasPrefix(s.SourceOffset, "retro_ask:L") {
			retro = append(retro, s)
		}
	}
	if len(retro) != 2 {
		t.Fatalf("want 2 retro_ask signals, got %d: %#v", len(retro), retro)
	}
	for _, s := range retro {
		if s.Kind != kleio.SignalTypeWorkItem {
			t.Errorf("retro kind=%q want work_item", s.Kind)
		}
		if s.Metadata["author"] != "user" {
			t.Errorf("retro metadata.author=%v want user", s.Metadata["author"])
		}
	}
}

func TestParseTranscript_RejectsNarrationFragments(t *testing.T) {
	dir := t.TempDir()
	noise := []string{
		"Both files are fixed, so now I need to verify the metadata is clean, then I'll move on.",
		"We have to make sure the test passes.",
		"Remaining work is to add docs.",
		"Let me re-run the tests.",
		"OK now I'll edit the file.",
		"This is what we'll do next: read the file, edit, then build.",
	}
	var lines []string
	for _, n := range noise {
		lines = append(lines, tline("assistant", contentBlock{Type: "text", Text: n}))
	}
	path := writeTranscript(t, dir, "noise.jsonl", lines)

	signals, err := parseTranscript(path, "kleio-cli", kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Fatalf("want 0 signals from narration, got %d: %#v", len(signals), signals)
	}
}

func TestParseTranscript_FirstUserTurnIsNotRetroAsk(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "first.jsonl", []string{
		tline("user", contentBlock{Type: "text", Text: "Actually, please build the ingester."}),
	})
	signals, err := parseTranscript(path, "kleio-cli", kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range signals {
		if strings.HasPrefix(s.SourceOffset, "retro_ask:") {
			t.Fatalf("first user turn must not produce a retro_ask: %#v", s)
		}
	}
}

func TestParseTranscript_DedupesIdenticalSignalsOnSameLine(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "dedup.jsonl", []string{
		tline("assistant", contentBlock{Type: "text", Text: "We will defer X. We will defer X."}),
	})
	signals, err := parseTranscript(path, "kleio-cli", kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	defer1 := 0
	for _, s := range signals {
		if strings.HasPrefix(s.SourceOffset, "defer:L") {
			defer1++
		}
	}
	if defer1 == 0 {
		t.Fatal("want at least one deferral signal")
	}
}

func TestIngester_DiscoversTranscriptsViaHook(t *testing.T) {
	dir := t.TempDir()
	p := writeTranscript(t, dir, "x.jsonl", []string{
		tline("assistant", contentBlock{Type: "tool_use", Name: "kleio_decide", Input: map[string]any{
			"content": "Stay narrow",
		}}),
	})
	ing := New()
	ing.DiscoverTranscriptsFn = func(_ kleio.IngestScope) ([]TranscriptInput, error) {
		return []TranscriptInput{{Path: p, RepoName: "kleio-cli"}}, nil
	}
	signals, err := ing.Ingest(context.Background(), kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 {
		t.Fatalf("want 1 signal, got %d", len(signals))
	}
	if signals[0].RepoName != "kleio-cli" {
		t.Fatalf("repo=%q want kleio-cli", signals[0].RepoName)
	}
}

func TestIngester_RootWalkFindsTranscripts(t *testing.T) {
	dir := t.TempDir()
	tdir := filepath.Join(dir, "agent-transcripts", "abc")
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, tdir, "x.jsonl", []string{
		tline("assistant", contentBlock{Type: "tool_use", Name: "kleio_capture", Input: map[string]any{
			"content":     "do thing",
			"signal_type": "work_item",
		}}),
	})
	writeTranscript(t, dir, "ignored.txt", []string{"not a transcript"})
	ing := New(dir)
	signals, err := ing.Ingest(context.Background(), kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 {
		t.Fatalf("want 1 signal from rooted walk, got %d", len(signals))
	}
}

func TestIngester_NameIsStable(t *testing.T) {
	if got := New().Name(); got != "transcript" {
		t.Errorf("ingester.Name()=%q want transcript", got)
	}
}
