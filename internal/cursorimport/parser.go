package cursorimport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var kleioTools = map[string]string{
	"kleio_decide":     "decision",
	"kleio_capture":    "work_item",
	"kleio_checkpoint": "checkpoint",
}

var fileTools = map[string]bool{
	"Write":      true,
	"StrReplace": true,
	"Read":       true,
}

// Heuristic patterns for implicit signal detection in assistant text.
var (
	decisionPhrases = []string{
		"i'll go with", "i'll use", "let's go with", "let's use",
		"we should use", "the best approach is", "i chose",
		"i've decided", "i decided", "going with",
		"my recommendation is", "i recommend",
		"between option", "after comparing",
		"instead of", "rather than",
	}
	todoPatterns = regexp.MustCompile(`(?i)\b(TODO|FIXME|HACK|XXX|FOLLOWUP|FOLLOW-UP|follow up)\b[:\s]`)
	workItemPatterns = regexp.MustCompile(`(?i)(need[s]? to|should|must|have to|will need to|requires?|we still need|remaining work|left to do|not yet implemented|out of scope for now|defer(?:red)?|punt(?:ed)?|skip(?:ped)? for now)`)
)

type TranscriptParser struct{}

func NewTranscriptParser() *TranscriptParser {
	return &TranscriptParser{}
}

func (p *TranscriptParser) ParseFile(path string) (*ParseResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read transcript: %w", err)
	}

	result, err := p.ParseLines(lines)
	if err != nil {
		return nil, err
	}
	result.FilePath = path
	return result, nil
}

func (p *TranscriptParser) ParseLines(lines []string) (*ParseResult, error) {
	result := &ParseResult{}
	seenHashes := make(map[string]bool)
	seenFiles := make(map[string]bool)
	hasKleioCalls := make(map[int]bool)

	// First pass: identify lines that contain explicit Kleio tool calls
	// so we can skip implicit extraction for those.
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var tl transcriptLine
		if err := json.Unmarshal([]byte(line), &tl); err != nil {
			continue
		}
		for _, block := range tl.Message.Content {
			if block.Type == "tool_use" {
				if _, ok := kleioTools[block.Name]; ok {
					hasKleioCalls[i] = true
				}
			}
		}
	}

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var tl transcriptLine
		if err := json.Unmarshal([]byte(line), &tl); err != nil {
			continue
		}

		switch tl.Role {
		case "user":
			result.UserTurns++
		case "assistant":
			result.AssistantTurns++
		}

		for _, block := range tl.Message.Content {
			if block.Type == "tool_use" {
				result.ToolCallCount++

				if signalType, ok := kleioTools[block.Name]; ok {
					sig := p.extractKleioSignal(block, signalType, i)
					hash := sig.Hash()
					if !seenHashes[hash] {
						seenHashes[hash] = true
						result.Signals = append(result.Signals, sig)
					}
				}

				if fileTools[block.Name] {
					if pathVal, ok := block.Input["path"]; ok {
						if pathStr, ok := pathVal.(string); ok && !seenFiles[pathStr] {
							seenFiles[pathStr] = true
							result.FilesTouched = append(result.FilesTouched, pathStr)
						}
					}
				}
			}

			// Implicit signal extraction from assistant text blocks.
			// Only extract from lines that don't already have a Kleio tool call,
			// so we don't double-count things the agent already captured.
			if block.Type == "text" && tl.Role == "assistant" && !hasKleioCalls[i] {
				for _, sig := range p.extractImplicitSignals(block.Text, i) {
					hash := sig.Hash()
					if !seenHashes[hash] {
						seenHashes[hash] = true
						result.Signals = append(result.Signals, sig)
					}
				}
			}

			// Implicit work items from user messages (TODO/follow-up requests).
			if block.Type == "text" && tl.Role == "user" {
				for _, sig := range p.extractUserWorkItems(block.Text, i) {
					hash := sig.Hash()
					if !seenHashes[hash] {
						seenHashes[hash] = true
						result.Signals = append(result.Signals, sig)
					}
				}
			}
		}
	}

	return result, nil
}

func (p *TranscriptParser) extractKleioSignal(block contentBlock, signalType string, lineOffset int) Signal {
	sig := Signal{
		SignalType:      signalType,
		AlreadyCaptured: true,
		LineOffset:      lineOffset,
	}

	if block.Name == "kleio_capture" {
		if st, ok := block.Input["signal_type"].(string); ok && st != "" {
			sig.SignalType = st
		}
	}

	if content, ok := block.Input["content"].(string); ok {
		sig.Content = content
	}
	if rationale, ok := block.Input["rationale"].(string); ok {
		sig.Rationale = rationale
	}

	return sig
}

// extractImplicitSignals detects decisions and work items from assistant
// prose that was NOT accompanied by a kleio_decide or kleio_capture call.
func (p *TranscriptParser) extractImplicitSignals(text string, lineOffset int) []Signal {
	if len(text) < 40 {
		return nil
	}

	var signals []Signal
	lower := strings.ToLower(text)
	sentences := splitSentences(text)

	// Implicit decisions: assistant discussing alternatives and choosing.
	for _, sent := range sentences {
		sentLower := strings.ToLower(sent)
		for _, phrase := range decisionPhrases {
			if strings.Contains(sentLower, phrase) && len(sent) >= 30 {
				signals = append(signals, Signal{
					SignalType:      "decision",
					Content:         truncateSentence(sent, 200),
					AlreadyCaptured: false,
					LineOffset:      lineOffset,
				})
				goto doneDecisions
			}
		}
	}
doneDecisions:

	// Implicit work items: TODO/FIXME/follow-up language.
	if todoPatterns.MatchString(text) {
		for _, sent := range sentences {
			if todoPatterns.MatchString(sent) && len(sent) >= 20 {
				signals = append(signals, Signal{
					SignalType:      "work_item",
					Content:         truncateSentence(sent, 200),
					AlreadyCaptured: false,
					LineOffset:      lineOffset,
				})
				break
			}
		}
	} else if workItemPatterns.MatchString(lower) {
		for _, sent := range sentences {
			if workItemPatterns.MatchString(strings.ToLower(sent)) && len(sent) >= 30 {
				signals = append(signals, Signal{
					SignalType:      "work_item",
					Content:         truncateSentence(sent, 200),
					AlreadyCaptured: false,
					LineOffset:      lineOffset,
				})
				break
			}
		}
	}

	return signals
}

// extractUserWorkItems detects explicit TODO/follow-up requests in user text.
func (p *TranscriptParser) extractUserWorkItems(text string, lineOffset int) []Signal {
	if len(text) < 20 || !todoPatterns.MatchString(text) {
		return nil
	}

	var signals []Signal
	for _, sent := range splitSentences(text) {
		if todoPatterns.MatchString(sent) && len(sent) >= 20 {
			signals = append(signals, Signal{
				SignalType:      "work_item",
				Content:         truncateSentence(sent, 200),
				AlreadyCaptured: false,
				LineOffset:      lineOffset,
			})
			break
		}
	}
	return signals
}

// splitSentences does a rough sentence split on periods, exclamation marks,
// and newlines. Good enough for heuristic extraction.
func splitSentences(text string) []string {
	text = strings.ReplaceAll(text, "\n", ". ")
	var sentences []string
	for _, raw := range strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	}) {
		s := strings.TrimSpace(raw)
		if s != "" {
			sentences = append(sentences, s)
		}
	}
	return sentences
}

func truncateSentence(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
