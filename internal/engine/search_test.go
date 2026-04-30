package engine

import (
	"context"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearch_RanksRecentHigher(t *testing.T) {
	eng, store := newTestEngine(t)
	ctx := context.Background()

	now := time.Now().UTC()

	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:         "old-1",
		SignalType: kleio.SignalTypeWorkItem,
		Content:    "auth: review login flow",
		SourceType: kleio.SourceTypeCLI,
		CreatedAt:  now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
	}))
	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:         "new-1",
		SignalType: kleio.SignalTypeWorkItem,
		Content:    "auth: fix token refresh",
		SourceType: kleio.SourceTypeCLI,
		CreatedAt:  now.Add(-1 * time.Hour).Format(time.RFC3339),
	}))

	results, err := eng.Search(ctx, "auth", 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 2)

	assert.Equal(t, "new-1", results[0].ID, "recent event should rank first")
}

func TestSearch_LimitResults(t *testing.T) {
	eng, store := newTestEngine(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
			SignalType: kleio.SignalTypeWorkItem,
			Content:    "auth related item",
			SourceType: kleio.SourceTypeCLI,
		}))
	}

	results, err := eng.Search(ctx, "auth", 3)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), 3)
}
