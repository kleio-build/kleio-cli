package localdb_test

import (
	"context"
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_CreateAndFindEntity(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	e := &kleio.Entity{
		Kind:            kleio.EntityKindTicket,
		Label:           "KLEIO-123",
		NormalizedLabel: "kleio-123",
		RepoName:        "kleio-cli",
	}
	require.NoError(t, s.CreateEntity(ctx, e))
	require.NotEmpty(t, e.ID)

	got, err := s.FindEntity(ctx, kleio.EntityKindTicket, "kleio-123")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, e.ID, got.ID)
	assert.Equal(t, "KLEIO-123", got.Label)
	assert.Equal(t, 1, got.MentionCount)
	assert.InDelta(t, 0.5, got.Confidence, 0.01)
}

func TestStore_FindEntity_NotFound(t *testing.T) {
	s := newTestStore(t)
	got, err := s.FindEntity(context.Background(), "ticket", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStore_CreateEntity_UpsertIncrementsMentionCount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	e := &kleio.Entity{
		ID:              "ent-1",
		Kind:            kleio.EntityKindFile,
		Label:           "auth/token.go",
		NormalizedLabel: "auth/token.go",
	}
	require.NoError(t, s.CreateEntity(ctx, e))

	// Second create with same ID should increment mention_count.
	e2 := &kleio.Entity{
		ID:              "ent-1",
		Kind:            kleio.EntityKindFile,
		Label:           "auth/token.go",
		NormalizedLabel: "auth/token.go",
	}
	require.NoError(t, s.CreateEntity(ctx, e2))

	got, err := s.FindEntity(ctx, kleio.EntityKindFile, "auth/token.go")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 2, got.MentionCount)
	// Confidence should have increased from the initial 0.5 default via the
	// upsert formula. The exact value depends on the SQL expression; we just
	// verify it moved upward from the base 0.3 floor used on conflict.
	assert.Greater(t, got.Confidence, 0.3)
}

func TestStore_ListEntities(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, e := range []*kleio.Entity{
		{Kind: kleio.EntityKindTicket, Label: "KLEIO-1", NormalizedLabel: "kleio-1"},
		{Kind: kleio.EntityKindTicket, Label: "KLEIO-2", NormalizedLabel: "kleio-2"},
		{Kind: kleio.EntityKindFile, Label: "main.go", NormalizedLabel: "main.go"},
	} {
		require.NoError(t, s.CreateEntity(ctx, e))
	}

	tickets, err := s.ListEntities(ctx, kleio.EntityFilter{Kind: kleio.EntityKindTicket})
	require.NoError(t, err)
	assert.Len(t, tickets, 2)

	all, err := s.ListEntities(ctx, kleio.EntityFilter{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestStore_CreateEntityAlias(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	e := &kleio.Entity{
		ID:              "ent-alias",
		Kind:            kleio.EntityKindFeature,
		Label:           "opengraph",
		NormalizedLabel: "opengraph",
	}
	require.NoError(t, s.CreateEntity(ctx, e))

	a := &kleio.EntityAlias{
		EntityID:   "ent-alias",
		Alias:      "og",
		Source:     kleio.AliasSourceCoOccurrence,
		Confidence: 0.75,
	}
	require.NoError(t, s.CreateEntityAlias(ctx, a))

	// Duplicate insert should not error (INSERT OR IGNORE).
	require.NoError(t, s.CreateEntityAlias(ctx, a))
}

func TestStore_CreateEntityMention_AndFindByEvidence(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	e := &kleio.Entity{
		ID:              "ent-m",
		Kind:            kleio.EntityKindTicket,
		Label:           "PROJ-42",
		NormalizedLabel: "proj-42",
	}
	require.NoError(t, s.CreateEntity(ctx, e))

	m := &kleio.EntityMention{
		EntityID:     "ent-m",
		EvidenceType: kleio.EvidenceTypeCommit,
		EvidenceID:   "commit-abc",
		Context:      "fix PROJ-42: handle nil pointer",
		Confidence:   0.9,
	}
	require.NoError(t, s.CreateEntityMention(ctx, m))
	require.NotEmpty(t, m.ID)

	entities, err := s.FindEntitiesByEvidence(ctx, "commit-abc")
	require.NoError(t, err)
	require.Len(t, entities, 1)
	assert.Equal(t, "PROJ-42", entities[0].Label)
}
