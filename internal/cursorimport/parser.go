package cursorimport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
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

