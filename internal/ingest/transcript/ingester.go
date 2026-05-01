// Package transcript implements a kleio.Ingester that walks Cursor agent
// transcripts (`agent-transcripts/*.jsonl`) and emits RawSignals from a
// strict accept-list ONLY:
//
//  1. Explicit kleio_capture / kleio_decide / kleio_checkpoint tool calls
//  2. Sentences containing explicit deferral language ("defer", "out of
//     scope", "punt", etc.) inside assistant messages
//  3. User retro-asks ("actually...", "wait...", "also can you...")
//
// Everything else is dropped. The previous implementation in
// internal/cursorimport/parser.go used per-sentence regex heuristics
// (decisionPhrases, workItemPatterns, todoPatterns) which produced
// hundreds of noisy work_items per transcript. This ingester is the
// narrow replacement.
package transcript

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

// Ingester implements kleio.Ingester for Cursor agent transcripts.
//
// Provenance: every emitted RawSignal carries SourceID equal to
// `<transcript-uuid>#<offset>` and SourceOffset matching one of the
// accepted patterns (`toolcall:Lxx`, `defer:Lxx`, `retro_ask:Lxx`).
// Auditors filter by SourceOffset prefix when verifying scope.
type Ingester struct {
	// Roots is the set of directories under which to discover transcripts.
	// Each root is searched for `agent-transcripts/**/*.jsonl`. When empty,
	// callers are expected to wire the ingester via DiscoverTranscriptsFn
	// (kept as a hook so the Phase 0 cursorimport.DiscoverTranscriptsScoped
	// remains the source of truth for scope resolution).
	Roots []string

	// DiscoverTranscriptsFn returns the list of transcript paths to ingest.
	// When non-nil it is called with the IngestScope and overrides Roots.
	// This indirection lets the CLI layer plug in the existing
	// cursorimport.DiscoverTranscriptsScoped helper (which knows about
	// CursorScope, --all-repos, .code-workspace, etc) without forcing
	// this package to depend on cursorimport.
	DiscoverTranscriptsFn func(scope kleio.IngestScope) ([]TranscriptInput, error)
}

// TranscriptInput is the unit returned by DiscoverTranscriptsFn: the
// transcript file path plus pre-resolved RepoName from scope discovery.
type TranscriptInput struct {
	Path     string
	RepoName string
}

func New(roots ...string) *Ingester { return &Ingester{Roots: roots} }

func (i *Ingester) Name() string { return "transcript" }

// Ingest discovers transcripts via DiscoverTranscriptsFn (when set) or
// the root walk (default), parses each one with parseTranscript, and
// returns the merged signals in deterministic order.
func (i *Ingester) Ingest(ctx context.Context, scope kleio.IngestScope) ([]kleio.RawSignal, error) {
	var inputs []TranscriptInput
	if i.DiscoverTranscriptsFn != nil {
		var err error
		inputs, err = i.DiscoverTranscriptsFn(scope)
		if err != nil {
			return nil, err
		}
	} else {
		for _, root := range i.Roots {
			inputs = append(inputs, walkRoot(root)...)
		}
	}

	sort.Slice(inputs, func(a, b int) bool { return inputs[a].Path < inputs[b].Path })

	var out []kleio.RawSignal
	for _, in := range inputs {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}
		signals, err := parseTranscript(in.Path, in.RepoName, scope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "transcript ingest: %s: %v\n", in.Path, err)
			continue
		}
		out = append(out, signals...)
	}
	return out, nil
}

// walkRoot finds every `*.jsonl` file under any `agent-transcripts`
// subdirectory of root. Used only when DiscoverTranscriptsFn is not set.
func walkRoot(root string) []TranscriptInput {
	var out []TranscriptInput
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}
		if !strings.Contains(filepath.ToSlash(path), "/agent-transcripts/") {
			return nil
		}
		out = append(out, TranscriptInput{Path: path})
		return nil
	})
	return out
}

// transcriptLine matches the JSON shape of one line in a Cursor
// agent-transcript JSONL.
type transcriptLine struct {
	Role    string `json:"role"`
	Message struct {
		Content []contentBlock `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

// kleioToolKinds maps the explicit Kleio tool-call name to the kleio
// signal_type it semantically represents. Anything outside this map is
// not treated as an explicit Kleio capture.
var kleioToolKinds = map[string]string{
	"kleio_capture":    kleio.SignalTypeWorkItem,
	"kleio_decide":     kleio.SignalTypeDecision,
	"kleio_checkpoint": kleio.SignalTypeCheckpoint,
}

// deferralRe matches explicit deferral language in assistant prose.
// Order matters: the most specific phrases appear first.
//
// We intentionally exclude soft phrases like "need to" / "have to" /
// "remaining work" -- those produced the noisy 815-signal baseline and
// are exactly what this ingester rejects.
// Note we allow up to ~60 chars between "skip(ping)" and "for now" so
// "Skipping the LLM correlator for now" matches but a paragraph apart
// does not.
var deferralRe = regexp.MustCompile(`(?i)\b(defer(?:red|ral|ring)?\b|out of scope\b|skip(?:ping|ped)?\b[^.\n]{0,60}\bfor now\b|punt(?:ed|ing)?\b|parked\b|won't (?:do|fix|tackle|build)\b|leave(?:s|d)? for (?:later|the next|a follow)\b|not in this (?:PR|session|change|sprint)\b)`)

// retroAskRe matches user retro-asks: a user turn (after the first one)
// that begins with one of the listed conversational pivots. These almost
// always introduce a new work item the agent needs to track.
var retroAskRe = regexp.MustCompile(`(?im)^\s*(actually|wait[,:]?|also[,:]?|and (?:please|can you))\b[^.!?\n]*[.!?\n]?`)

// parseTranscript walks one JSONL file once, applying the three accept
// rules. Signals from the same line are deduplicated by hash so a single
// noisy assistant turn cannot emit dozens of near-duplicate signals.
func parseTranscript(path, repoName string, scope kleio.IngestScope) ([]kleio.RawSignal, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	st, _ := f.Stat()
	var modTime time.Time
	if st != nil {
		modTime = st.ModTime()
	}
	if !scope.Since.IsZero() && modTime.Before(scope.Since) {
		return nil, nil
	}

	transcriptUUID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	var out []kleio.RawSignal
	seen := map[string]bool{}
	emit := func(s kleio.RawSignal) {
		key := s.SourceID + "|" + s.Content
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}

	lineNo := 0
	userTurnsSeen := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var tl transcriptLine
		if err := json.Unmarshal([]byte(raw), &tl); err != nil {
			continue
		}
		isUserTurn := tl.Role == "user"
		if isUserTurn {
			userTurnsSeen++
		}
		for _, block := range tl.Message.Content {
			switch block.Type {
			case "tool_use":
				toolName, args := normalizeToolUse(block)
				kind, ok := kleioToolKinds[toolName]
				if !ok {
					continue
				}
				content := stringInput(args, "content")
				if content == "" {
					continue
				}
				if name := stringInput(args, "signal_type"); toolName == "kleio_capture" && name != "" {
					kind = name
				}
				md := map[string]any{
					"transcript_uuid": transcriptUUID,
					"tool_name":       toolName,
					"line":            lineNo,
				}
				if rat := stringInput(args, "rationale"); rat != "" {
					md["rationale"] = rat
				}
				emit(kleio.RawSignal{
					SourceType:   kleio.SourceTypeCursorTranscript,
					SourceID:     fmt.Sprintf("%s#toolcall:L%d", transcriptUUID, lineNo),
					SourceOffset: fmt.Sprintf("toolcall:L%d", lineNo),
					Content:      content,
					Kind:         kind,
					Timestamp:    modTime,
					RepoName:     repoName,
					Metadata:     md,
				})
			case "text":
				if tl.Role == "assistant" {
					for _, m := range deferralRe.FindAllStringIndex(block.Text, -1) {
						snippet := captureSentence(block.Text, m[0], m[1])
						if snippet == "" {
							continue
						}
						emit(kleio.RawSignal{
							SourceType:   kleio.SourceTypeCursorTranscript,
							SourceID:     fmt.Sprintf("%s#defer:L%d:%d", transcriptUUID, lineNo, m[0]),
							SourceOffset: fmt.Sprintf("defer:L%d", lineNo),
							Content:      snippet,
							Kind:         kleio.SignalTypeWorkItem,
							Timestamp:    modTime,
							RepoName:     repoName,
							Metadata: map[string]any{
								"transcript_uuid": transcriptUUID,
								"line":            lineNo,
								"deferred":        true,
							},
						})
					}
				}
				if isUserTurn && userTurnsSeen > 1 {
					for _, m := range retroAskRe.FindAllStringIndex(block.Text, -1) {
						snippet := captureSentence(block.Text, m[0], m[1])
						if snippet == "" || len(snippet) < 30 || isConversationalNoise(snippet) {
							continue
						}
						emit(kleio.RawSignal{
							SourceType:   kleio.SourceTypeCursorTranscript,
							SourceID:     fmt.Sprintf("%s#retro_ask:L%d:%d", transcriptUUID, lineNo, m[0]),
							SourceOffset: fmt.Sprintf("retro_ask:L%d", lineNo),
							Content:      snippet,
							Kind:         kleio.SignalTypeWorkItem,
							Timestamp:    modTime,
							RepoName:     repoName,
							Metadata: map[string]any{
								"transcript_uuid": transcriptUUID,
								"line":            lineNo,
								"author":          "user",
							},
						})
					}
				}
			}
		}
	}
	return out, scanner.Err()
}

// captureSentence returns the surrounding sentence of a regex match,
// trimmed and length-capped. Sentences are bounded by '.', '!', '?', or
// newline; the snippet always includes the matched span.
func captureSentence(text string, matchStart, matchEnd int) string {
	const cap = 220
	start := matchStart
	for start > 0 {
		c := text[start-1]
		if c == '.' || c == '!' || c == '?' || c == '\n' {
			break
		}
		start--
	}
	end := matchEnd
	for end < len(text) {
		c := text[end]
		if c == '.' || c == '!' || c == '?' || c == '\n' {
			end++
			break
		}
		end++
	}
	snippet := strings.TrimSpace(text[start:end])
	if len(snippet) > cap {
		snippet = snippet[:cap-3] + "..."
	}
	return snippet
}

// normalizeToolUse returns the effective tool name and argument map for
// a `tool_use` block. Direct-use blocks (e.g. {"name": "Write", "input":
// {...}}) pass through unchanged. CallMcpTool blocks are unwrapped one
// level: the MCP toolName becomes the effective name, and the
// `arguments` object becomes the effective input. This is what makes
// kleio_* MCP calls visible to the ingester (Cursor routes all MCP
// tools through CallMcpTool, never as bare tool_use blocks).
func normalizeToolUse(block contentBlock) (string, map[string]any) {
	if block.Name != "CallMcpTool" {
		return block.Name, block.Input
	}
	toolName := stringInput(block.Input, "toolName")
	args, _ := block.Input["arguments"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}
	return toolName, args
}

func stringInput(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// conversationalNoiseRe matches retro-ask snippets that are purely
// conversational with no actionable content. These pass the retro_ask
// regex but add no signal value.
var conversationalNoiseRe = regexp.MustCompile(`(?i)^(wait[,:]?\s+(what|so|but|hold on|huh)|actually[,:]?\s+(never ?mind|no|nah|hmm)|also[,:]?\s+(can you share|I (was|am) (wondering|confused|curious)))`)

func isConversationalNoise(snippet string) bool {
	if conversationalNoiseRe.MatchString(snippet) && len(snippet) < 60 {
		return true
	}
	words := strings.Fields(snippet)
	if len(words) <= 4 {
		return true
	}
	return false
}
