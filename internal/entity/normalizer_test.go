package entity_test

import (
	"context"
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/entity"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *localdb.Store {
	t.Helper()
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return localdb.New(db)
}

func TestNormalizer_PersistSignalEntities(t *testing.T) {
	store := newTestStore(t)
	norm := entity.NewNormalizer(store)
	ctx := context.Background()

	signals := []kleio.RawSignal{
		{
			SourceType: kleio.SourceTypeLocalGit,
			SourceID:   "git:kleio-cli:abc123",
			Content:    "fix KLEIO-42: auth token refresh",
			RepoName:   "kleio-cli",
			Metadata: map[string]any{
				"extracted_entities": []entity.ExtractedEntity{
					{Kind: kleio.EntityKindTicket, Value: "KLEIO-42", Confidence: 0.95, Source: "commit_message"},
				},
			},
		},
		{
			SourceType: "cursor_plan",
			SourceID:   "plan:auth_refactor_abc12345",
			Content:    "Fix KLEIO-42 auth handling",
			RepoName:   "kleio-cli",
			Metadata: map[string]any{
				"extracted_entities": []entity.ExtractedEntity{
					{Kind: kleio.EntityKindTicket, Value: "KLEIO-42", Confidence: 0.9, Source: "plan"},
				},
			},
		},
	}

	created, err := norm.PersistSignalEntities(ctx, signals)
	require.NoError(t, err)
	assert.Equal(t, 2, created)

	// Entity should exist with mention_count 2 (upserted).
	ent, err := store.FindEntity(ctx, kleio.EntityKindTicket, "KLEIO-42")
	require.NoError(t, err)
	require.NotNil(t, ent)
	assert.Equal(t, 2, ent.MentionCount)

	// Both mentions should be linked.
	entities, err := store.FindEntitiesByEvidence(ctx, "git:kleio-cli:abc123")
	require.NoError(t, err)
	assert.Len(t, entities, 1)

	entities2, err := store.FindEntitiesByEvidence(ctx, "plan:auth_refactor_abc12345")
	require.NoError(t, err)
	assert.Len(t, entities2, 1)
}

func TestNormalizer_CaseInsensitiveDedup(t *testing.T) {
	store := newTestStore(t)
	norm := entity.NewNormalizer(store)
	ctx := context.Background()

	signals := []kleio.RawSignal{
		{
			SourceID: "sig-1",
			Content:  "internal/auth/token.go",
			Metadata: map[string]any{
				"extracted_entities": []entity.ExtractedEntity{
					{Kind: kleio.EntityKindFile, Value: "internal/auth/token.go", Confidence: 0.8, Source: "plan"},
				},
			},
		},
		{
			SourceID: "sig-2",
			Content:  "internal/Auth/Token.go",
			Metadata: map[string]any{
				"extracted_entities": []entity.ExtractedEntity{
					{Kind: kleio.EntityKindFile, Value: "internal/Auth/Token.go", Confidence: 0.8, Source: "commit_message"},
				},
			},
		},
	}

	_, err := norm.PersistSignalEntities(ctx, signals)
	require.NoError(t, err)

	// Should be one entity (case-insensitive dedup), mention_count 2.
	all, err := store.ListEntities(ctx, kleio.EntityFilter{Kind: kleio.EntityKindFile})
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, 2, all[0].MentionCount)
}

func TestNormalizer_UpsertIdempotency(t *testing.T) {
	store := newTestStore(t)
	norm := entity.NewNormalizer(store)
	ctx := context.Background()

	signals := []kleio.RawSignal{
		{
			SourceID: "sig-1",
			Content:  "PROJ-99",
			Metadata: map[string]any{
				"extracted_entities": []entity.ExtractedEntity{
					{Kind: kleio.EntityKindTicket, Value: "PROJ-99", Confidence: 0.9, Source: "branch"},
				},
			},
		},
	}

	// Run twice -- should not create duplicates.
	_, err := norm.PersistSignalEntities(ctx, signals)
	require.NoError(t, err)
	_, err = norm.PersistSignalEntities(ctx, signals)
	require.NoError(t, err)

	all, err := store.ListEntities(ctx, kleio.EntityFilter{Kind: kleio.EntityKindTicket})
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, 2, all[0].MentionCount) // 1 initial + 1 upsert
}
