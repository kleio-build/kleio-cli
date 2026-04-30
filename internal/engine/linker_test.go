package engine

import (
	"context"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoLink_SHAMention(t *testing.T) {
	eng, store := newTestEngine(t)
	ctx := context.Background()

	now := time.Now().UTC().Format(time.RFC3339)

	require.NoError(t, store.IndexCommits(ctx, "/repo", []kleio.Commit{
		{
			SHA:         "abc1234567890",
			RepoPath:    "/repo",
			Message:     "feat: add auth",
			CommittedAt: now,
		},
	}))

	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:         "evt-1",
		SignalType: kleio.SignalTypeWorkItem,
		Content:    "relates to commit abc1234",
		SourceType: kleio.SourceTypeCLI,
		CreatedAt:  now,
	}))

	created, err := eng.AutoLink(ctx)
	require.NoError(t, err)
	assert.Greater(t, created, 0)

	links, err := store.QueryLinks(ctx, kleio.LinkFilter{SourceID: "evt-1"})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(links), 1)
}

func TestAutoLink_NoMatches(t *testing.T) {
	eng, store := newTestEngine(t)
	ctx := context.Background()

	now := time.Now().UTC().Format(time.RFC3339)

	require.NoError(t, store.IndexCommits(ctx, "/repo", []kleio.Commit{
		{SHA: "zzz999", RepoPath: "/repo", Message: "unrelated", CommittedAt: now},
	}))

	require.NoError(t, store.CreateEvent(ctx, &kleio.Event{
		ID:         "evt-2",
		SignalType: kleio.SignalTypeWorkItem,
		Content:    "something completely different",
		SourceType: kleio.SourceTypeCLI,
		CreatedAt:  now,
	}))

	created, err := eng.AutoLink(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, created)
}
