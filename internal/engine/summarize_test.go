package engine

import (
	"context"
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummarize_Heuristic_Empty(t *testing.T) {
	eng, _ := newTestEngine(t)
	result, err := eng.Summarize(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "No events to summarize.", result)
}

func TestSummarize_Heuristic_Mixed(t *testing.T) {
	eng, _ := newTestEngine(t)
	events := []kleio.Event{
		{SignalType: kleio.SignalTypeDecision, Content: "decided to use JWT"},
		{SignalType: kleio.SignalTypeCheckpoint, Content: "auth module done"},
		{SignalType: kleio.SignalTypeWorkItem, Content: "fix login bug"},
		{SignalType: kleio.SignalTypeWorkItem, Content: "add password reset"},
	}

	result, err := eng.Summarize(context.Background(), events)
	require.NoError(t, err)
	assert.Contains(t, result, "4 event(s)")
	assert.Contains(t, result, "1 decision(s)")
	assert.Contains(t, result, "1 checkpoint(s)")
	assert.Contains(t, result, "2 work item(s)")
}

func TestSummarize_Heuristic_IncludesMostRecent(t *testing.T) {
	eng, _ := newTestEngine(t)
	events := []kleio.Event{
		{SignalType: kleio.SignalTypeDecision, Content: "the most recent event"},
	}

	result, err := eng.Summarize(context.Background(), events)
	require.NoError(t, err)
	assert.Contains(t, result, "the most recent event")
}
