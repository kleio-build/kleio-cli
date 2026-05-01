package entity

import (
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtract_Tickets(t *testing.T) {
	text := "Fix KLEIO-123 and PROJ-42, see KL-7"
	entities := Extract(text, "commit_message")

	var tickets []string
	for _, e := range entities {
		if e.Kind == kleio.EntityKindTicket {
			tickets = append(tickets, e.Value)
		}
	}
	assert.Contains(t, tickets, "KL-7")
	assert.Contains(t, tickets, "KLEIO-123")
	assert.Contains(t, tickets, "PROJ-42")
}

func TestExtract_FilePaths(t *testing.T) {
	text := "Changed internal/auth/token.go and ./cmd/main.go"
	entities := Extract(text, "plan")

	var files []string
	for _, e := range entities {
		if e.Kind == kleio.EntityKindFile {
			files = append(files, e.Value)
		}
	}
	assert.Contains(t, files, "internal/auth/token.go")
	assert.Contains(t, files, "cmd/main.go")
}

func TestExtract_PlanAnchors(t *testing.T) {
	text := "See plan report_quality_fixes_de202626"
	entities := Extract(text, "transcript")

	found := false
	for _, e := range entities {
		if e.Kind == kleio.EntityKindPlanAnchor && e.Value == "report_quality_fixes_de202626" {
			found = true
		}
	}
	assert.True(t, found, "should extract plan anchor")
}

func TestExtract_Deduplicates(t *testing.T) {
	text := "Fix KLEIO-123 and also KLEIO-123 again"
	entities := Extract(text, "commit_message")

	count := 0
	for _, e := range entities {
		if e.Value == "KLEIO-123" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestExtract_GitHubPR(t *testing.T) {
	text := "Merge pull request #42 from feature/auth"
	entities := Extract(text, "commit_message")

	found := false
	for _, e := range entities {
		if e.Kind == kleio.EntityKindTicket && e.Value == "#42" {
			found = true
		}
	}
	assert.True(t, found, "should extract GitHub PR reference")
}

func TestExtractDecisionNames(t *testing.T) {
	text := "## Use JWT for authentication\n\n## Setup\n\n### Strategy: local-first pivot"
	entities := ExtractDecisionNames(text, "plan")

	require.Len(t, entities, 2)
	assert.Equal(t, "Use JWT for authentication", entities[0].Value)
	assert.Equal(t, "Strategy: local-first pivot", entities[1].Value)
}

func TestNormalizePath(t *testing.T) {
	assert.Equal(t, "cmd/main.go", NormalizePath("./cmd/main.go"))
	assert.Equal(t, "internal/auth/token.go", NormalizePath("internal\\auth\\token.go"))
}

func TestNormalizeLabel(t *testing.T) {
	assert.Equal(t, "KLEIO-123", NormalizeLabel(kleio.EntityKindTicket, "kleio-123"))
	assert.Equal(t, "internal/auth/token.go", NormalizeLabel(kleio.EntityKindFile, "./internal/auth/token.go"))
	assert.Equal(t, "validatetoken", NormalizeLabel(kleio.EntityKindSymbol, "ValidateToken"))
}
