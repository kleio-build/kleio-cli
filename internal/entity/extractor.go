// Package entity provides a shared EntityExtractor that scans raw text
// for mentions of tickets, file paths, plan anchors, and other entities.
// Used by all ingesters to attach extracted entities to RawSignal.Metadata.
package entity

import (
	"path/filepath"
	"regexp"
	"strings"

	kleio "github.com/kleio-build/kleio-core"
)

// ExtractedEntity is a single entity mention found in text.
type ExtractedEntity struct {
	Kind       string  `json:"kind"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
}

var (
	jiraRe       = regexp.MustCompile(`\b([A-Z][A-Z0-9]+-\d+)\b`)
	kleioTicketRe = regexp.MustCompile(`\b(KL-\d+)\b`)
	githubPRRe   = regexp.MustCompile(`(?:^|[\s(])#(\d{1,5})\b`)
	planAnchorRe = regexp.MustCompile(`\b([a-z][a-z0-9_-]+_[0-9a-f]{8})\b`)
	inlinePathRe = regexp.MustCompile(`(?:\.[\\/]|[\w\-]+[\\/])(?:[\w\-]+[\\/])*[\w\-]+\.(go|ts|tsx|js|jsx|py|rs|rb|md|sql|sh|yaml|yml|json|toml|html|css)\b`)
	headingRe    = regexp.MustCompile(`(?m)^#{1,4}\s+(.+?)\s*$`)
)

// Extract scans text for entity mentions. The source parameter identifies
// where the text came from (branch, commit_message, plan, transcript).
func Extract(text, source string) []ExtractedEntity {
	if text == "" {
		return nil
	}

	seen := map[string]bool{}
	var out []ExtractedEntity

	add := func(kind, value string, confidence float64) {
		key := kind + ":" + value
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, ExtractedEntity{
			Kind:       kind,
			Value:      value,
			Confidence: confidence,
			Source:      source,
		})
	}

	for _, m := range kleioTicketRe.FindAllStringSubmatch(text, -1) {
		add(kleio.EntityKindTicket, m[1], 0.95)
	}
	for _, m := range jiraRe.FindAllStringSubmatch(text, -1) {
		if kleioTicketRe.MatchString(m[1]) {
			continue
		}
		add(kleio.EntityKindTicket, m[1], 0.9)
	}
	for _, m := range githubPRRe.FindAllStringSubmatch(text, -1) {
		add(kleio.EntityKindTicket, "#"+m[1], 0.85)
	}
	for _, m := range planAnchorRe.FindAllStringSubmatch(text, -1) {
		add(kleio.EntityKindPlanAnchor, m[1], 0.9)
	}
	for _, m := range inlinePathRe.FindAllString(text, -1) {
		normalized := NormalizePath(m)
		add(kleio.EntityKindFile, normalized, 0.8)
	}

	return out
}

// ExtractDecisionNames pulls heading-like decision names from plan text.
func ExtractDecisionNames(text, source string) []ExtractedEntity {
	var out []ExtractedEntity
	seen := map[string]bool{}
	for _, m := range headingRe.FindAllStringSubmatch(text, -1) {
		name := strings.TrimSpace(m[1])
		if len(name) < 5 || len(strings.Fields(name)) < 2 {
			continue
		}
		lower := strings.ToLower(name)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		out = append(out, ExtractedEntity{
			Kind:       kleio.EntityKindDecisionName,
			Value:      name,
			Confidence: 0.7,
			Source:      source,
		})
	}
	return out
}

// NormalizePath strips leading ./ and normalizes separators.
func NormalizePath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "./")
	return p
}

// NormalizeLabel returns a canonical lowercase form for entity matching.
func NormalizeLabel(kind, value string) string {
	switch kind {
	case kleio.EntityKindFile:
		return NormalizePath(strings.ToLower(value))
	case kleio.EntityKindTicket:
		return strings.ToUpper(value)
	default:
		return strings.ToLower(value)
	}
}
