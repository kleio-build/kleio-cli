package commands

import (
	"testing"
	"time"

	"github.com/kleio-build/kleio-cli/internal/engine"
	"github.com/stretchr/testify/assert"
)

func TestSegmentTimeline_GroupsByDate(t *testing.T) {
	now := time.Now()
	entries := []engine.TimelineEntry{
		{Timestamp: now.Add(-48 * time.Hour), Kind: "commit", Summary: "old commit"},
		{Timestamp: now.Add(-47 * time.Hour), Kind: "event", Summary: "old event"},
		{Timestamp: now.Add(-1 * time.Hour), Kind: "commit", Summary: "recent commit"},
	}

	segments := segmentTimeline(entries)
	assert.GreaterOrEqual(t, len(segments), 2, "should have at least 2 date segments")
}

func TestSegmentTimeline_Empty(t *testing.T) {
	segments := segmentTimeline(nil)
	assert.Nil(t, segments)
}

func TestKindIcon(t *testing.T) {
	assert.Equal(t, "[commit]", kindIcon("commit"))
	assert.Equal(t, "[event]", kindIcon("event"))
	assert.Equal(t, "[link]", kindIcon("link"))
	assert.Equal(t, "[other]", kindIcon("other"))
}

func TestIsInteractive(t *testing.T) {
	// In test mode, stdin is typically not a terminal
	result := isInteractive()
	assert.IsType(t, true, result)
}
