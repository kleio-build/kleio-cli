package cursorimport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type Signal struct {
	SignalType      string
	Content         string
	Rationale       string
	Confidence      float64
	AlreadyCaptured bool
	LineOffset      int
	SourceFile      string
}

// Hash produces a stable dedup key based on signal type and content.
// Line offset is excluded so identical signals at different lines are deduped.
func (s Signal) Hash() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%s", s.SignalType, s.Content)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

type ParseResult struct {
	FilePath       string
	Signals        []Signal
	FilesTouched   []string
	UserTurns      int
	AssistantTurns int
	ToolCallCount  int
}

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
