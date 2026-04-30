package localdb

import (
	"context"
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListUnsyncedEvents(t *testing.T) {
	db, err := OpenInMemory()
	require.NoError(t, err)
	defer db.Close()
	store := New(db)
	ctx := context.Background()

	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:         "evt-1",
		Content:    "unsynced event",
		SignalType: kleio.SignalTypeWorkItem,
		SourceType: kleio.SourceTypeCLI,
	}))

	events, err := store.ListUnsyncedEvents(ctx)
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "evt-1", events[0].ID)
}

func TestMarkEventSynced(t *testing.T) {
	db, err := OpenInMemory()
	require.NoError(t, err)
	defer db.Close()
	store := New(db)
	ctx := context.Background()

	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:         "evt-2",
		Content:    "to be synced",
		SignalType: kleio.SignalTypeWorkItem,
		SourceType: kleio.SourceTypeCLI,
	}))

	require.NoError(t, store.MarkEventSynced(ctx, "evt-2"))

	events, err := store.ListUnsyncedEvents(ctx)
	require.NoError(t, err)
	assert.Len(t, events, 0)

	synced, err := store.GetEvent(ctx, "evt-2")
	require.NoError(t, err)
	assert.True(t, synced.Synced)
}

func TestListUnsyncedBacklogItems(t *testing.T) {
	db, err := OpenInMemory()
	require.NoError(t, err)
	defer db.Close()
	store := New(db)
	ctx := context.Background()

	require.NoError(t, store.CreateBacklogItem(ctx, &kleio.BacklogItem{
		ID:    "bl-1",
		Title: "unsynced item",
	}))

	items, err := store.ListUnsyncedBacklogItems(ctx)
	require.NoError(t, err)
	assert.Len(t, items, 1)
}

func TestMarkBacklogItemSynced(t *testing.T) {
	db, err := OpenInMemory()
	require.NoError(t, err)
	defer db.Close()
	store := New(db)
	ctx := context.Background()

	require.NoError(t, store.CreateBacklogItem(ctx, &kleio.BacklogItem{
		ID:    "bl-2",
		Title: "to sync",
	}))

	require.NoError(t, store.MarkBacklogItemSynced(ctx, "bl-2"))

	items, err := store.ListUnsyncedBacklogItems(ctx)
	require.NoError(t, err)
	assert.Len(t, items, 0)
}

func TestMarkEventSynced_NotFound(t *testing.T) {
	db, err := OpenInMemory()
	require.NoError(t, err)
	defer db.Close()
	store := New(db)

	err = store.MarkEventSynced(context.Background(), "nonexistent")
	assert.Error(t, err)
}
