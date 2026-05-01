package entity

import (
	"context"
	"strings"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/google/uuid"
)

// Normalizer resolves extracted entities from RawSignals into canonical
// entities in the store. On repeated ingest, existing entities have their
// mention_count incremented and confidence recalculated. Co-occurrence
// aliases are learned when two distinct surface forms appear together in
// enough shared evidence.
type Normalizer struct {
	Store            kleio.Store
	CoOccurrenceMin  int // minimum shared evidence to create alias (default 3)
}

// NewNormalizer creates a Normalizer with default thresholds.
func NewNormalizer(store kleio.Store) *Normalizer {
	return &Normalizer{Store: store, CoOccurrenceMin: 3}
}

// PersistSignalEntities processes all signals from an ingest run,
// upserting entities and creating mentions. Returns the number of
// entity-mention links created.
func (n *Normalizer) PersistSignalEntities(ctx context.Context, signals []kleio.RawSignal) (int, error) {
	mentionsCreated := 0

	for _, sig := range signals {
		raw, ok := sig.Metadata["extracted_entities"]
		if !ok {
			continue
		}
		entities, ok := raw.([]ExtractedEntity)
		if !ok {
			continue
		}
		for _, ext := range entities {
			entID, err := n.upsertEntity(ctx, ext, sig.RepoName)
			if err != nil {
				continue
			}
			m := &kleio.EntityMention{
				EntityID:     entID,
				EvidenceType: evidenceTypeFromSignal(sig),
				EvidenceID:   sig.SourceID,
				Context:      truncate(sig.Content, 200),
				Confidence:   ext.Confidence,
			}
			if err := n.Store.CreateEntityMention(ctx, m); err == nil {
				mentionsCreated++
			}
		}
	}
	return mentionsCreated, nil
}

// upsertEntity finds or creates a canonical entity for the extracted mention.
func (n *Normalizer) upsertEntity(ctx context.Context, ext ExtractedEntity, repoName string) (string, error) {
	normalized := NormalizeLabel(ext.Kind, ext.Value)

	existing, err := n.Store.FindEntity(ctx, ext.Kind, normalized)
	if err != nil {
		return "", err
	}
	if existing != nil {
		// Re-insert to trigger upsert (increment mention_count).
		dup := *existing
		if err := n.Store.CreateEntity(ctx, &dup); err != nil {
			return "", err
		}
		// If the surface form differs from the label, record as alias.
		if !strings.EqualFold(ext.Value, existing.Label) {
			_ = n.Store.CreateEntityAlias(ctx, &kleio.EntityAlias{
				EntityID:   existing.ID,
				Alias:      ext.Value,
				Source:      ext.Source,
				Confidence: ext.Confidence,
			})
		}
		return existing.ID, nil
	}

	// New entity.
	e := &kleio.Entity{
		ID:              uuid.NewString(),
		Kind:            ext.Kind,
		Label:           ext.Value,
		NormalizedLabel: normalized,
		RepoName:        repoName,
		Confidence:      ext.Confidence,
		MentionCount:    1,
	}
	if err := n.Store.CreateEntity(ctx, e); err != nil {
		return "", err
	}
	return e.ID, nil
}

// LearnCoOccurrenceAliases scans entity_mentions for entities that
// co-occur in the same evidence. When two entities of the same kind
// share >= CoOccurrenceMin evidence IDs, they are linked as aliases.
func (n *Normalizer) LearnCoOccurrenceAliases(ctx context.Context) (int, error) {
	// This is a post-ingest batch step. We use the store to find
	// co-occurring entities via SQL (implemented in localdb).
	localStore, ok := n.Store.(CoOccurrenceQuerier)
	if !ok {
		return 0, nil
	}
	threshold := n.CoOccurrenceMin
	if threshold <= 0 {
		threshold = 3
	}
	return localStore.LearnCoOccurrenceAliases(ctx, threshold)
}

// CoOccurrenceQuerier is an optional interface for stores that support
// batch co-occurrence alias learning.
type CoOccurrenceQuerier interface {
	LearnCoOccurrenceAliases(ctx context.Context, threshold int) (int, error)
}

func evidenceTypeFromSignal(sig kleio.RawSignal) string {
	switch sig.SourceType {
	case kleio.SourceTypeLocalGit:
		return kleio.EvidenceTypeCommit
	default:
		return kleio.EvidenceTypeSignal
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
