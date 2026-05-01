// Package entityoverlap implements a kleio.Correlator that clusters
// RawSignals sharing normalized entities. It queries the entity_mentions
// table to find signals linked to the same entity and emits Clusters
// with confidence proportional to the entity kind: tickets (0.85),
// files (0.70), features (0.60).
package entityoverlap

import (
	"context"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/entity"
)

var kindConfidence = map[string]float64{
	kleio.EntityKindTicket:       0.85,
	kleio.EntityKindFile:         0.70,
	kleio.EntityKindSymbol:       0.70,
	kleio.EntityKindFeature:      0.60,
	kleio.EntityKindDecisionName: 0.60,
	kleio.EntityKindPlanAnchor:   0.80,
	kleio.EntityKindActor:        0.50,
}

// Correlator clusters signals sharing entities from the entity graph.
type Correlator struct {
	Store kleio.Store
}

func New(store kleio.Store) *Correlator {
	return &Correlator{Store: store}
}

func (c *Correlator) Name() string { return "entity_overlap" }

// Correlate finds signals sharing entities and groups them into clusters.
// The cluster anchor is the signal with the highest entity confidence.
func (c *Correlator) Correlate(ctx context.Context, signals []kleio.RawSignal) ([]kleio.Cluster, error) {

	// Build a map: entity_id -> list of signals mentioning it.
	type entityInfo struct {
		kind      string
		signalIDs []string
	}
	entitySignals := map[string]*entityInfo{}

	signalByID := map[string]kleio.RawSignal{}
	for _, sig := range signals {
		signalByID[sig.SourceID] = sig

		raw, ok := sig.Metadata["extracted_entities"]
		if !ok {
			continue
		}
		var entities []entity.ExtractedEntity
		switch v := raw.(type) {
		case []entity.ExtractedEntity:
			entities = v
		default:
			continue
		}
		if len(entities) == 0 {
			continue
		}
		for _, ext := range entities {
			normalized := entity.NormalizeLabel(ext.Kind, ext.Value)
			key := ext.Kind + ":" + normalized

			info, exists := entitySignals[key]
			if !exists {
				info = &entityInfo{kind: ext.Kind}
				entitySignals[key] = info
			}
			// Deduplicate signal IDs per entity.
			found := false
			for _, id := range info.signalIDs {
				if id == sig.SourceID {
					found = true
					break
				}
			}
			if !found {
				info.signalIDs = append(info.signalIDs, sig.SourceID)
			}
		}
	}

	// For each entity shared by 2+ signals, emit a cluster.
	seen := map[string]bool{}
	var clusters []kleio.Cluster

	for _, info := range entitySignals {
		if len(info.signalIDs) < 2 {
			continue
		}

		conf := kindConfidence[info.kind]
		if conf == 0 {
			conf = 0.50
		}

		// Use the first signal as anchor.
		anchorID := info.signalIDs[0]
		sigKey := clusterSig(info.signalIDs)
		if seen[sigKey] {
			continue
		}
		seen[sigKey] = true

		var members []kleio.RawSignal
		for _, sid := range info.signalIDs {
			if sig, ok := signalByID[sid]; ok {
				members = append(members, sig)
			}
		}

		if len(members) < 2 {
			continue
		}

		cluster := kleio.Cluster{
			AnchorID:   anchorID,
			AnchorType: "entity_overlap",
			Members:    members,
			Confidence: conf,
			Provenance: []string{"entity_overlap"},
		}
		clusters = append(clusters, cluster)
	}

	return clusters, nil
}

func clusterSig(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	s := ids[0]
	for i := 1; i < len(ids); i++ {
		s += "|" + ids[i]
	}
	return s
}
