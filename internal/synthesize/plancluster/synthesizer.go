// Package plancluster implements a kleio.Synthesizer for clusters
// anchored on a Cursor plan signal. It emits one Event per
// signal-promoted-to-capture: the umbrella plan becomes a checkpoint;
// frontmatter todos become work_items; explicit decision blocks
// become decisions; out-of-scope/deferred items become work_items
// flagged deferred.
//
// Why plan-anchored only? Plans are the user-authored ground truth
// per their guidance ("Plans are the most valuable... we still need
// to parse transcripts regardless"). Cluster anchors that are NOT
// plans get routed to OrphanSynthesizer (Phase 4.2), which is far
// more conservative.
package plancluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

// Synthesizer implements kleio.Synthesizer for plan-anchored clusters.
type Synthesizer struct{}

func New() *Synthesizer { return &Synthesizer{} }

func (*Synthesizer) Name() string { return "plan_cluster" }

// Synthesize emits one Event per cluster member that should become a
// persistent capture. Cluster members that are NOT plan-derived
// (transcripts, commits) get attached to the resulting Events as
// provenance metadata but never become Events themselves -- this
// synthesizer's responsibility is the plan story, not its supporting
// evidence.
//
// Returns no Events when the cluster's anchor isn't a plan signal;
// the OrphanSynthesizer handles those.
func (*Synthesizer) Synthesize(ctx context.Context, cluster kleio.Cluster) ([]kleio.Event, error) {
	if cluster.AnchorType != "cursor_plan" {
		return nil, nil
	}

	var anchor *kleio.RawSignal
	for i := range cluster.Members {
		if cluster.Members[i].SourceID == cluster.AnchorID {
			anchor = &cluster.Members[i]
			break
		}
	}
	if anchor == nil {
		return nil, nil
	}

	supportingIDs := collectSupporting(cluster, cluster.AnchorID)

	var events []kleio.Event
	for _, m := range cluster.Members {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if m.SourceType != "cursor_plan" {
			continue
		}
		ev, ok := buildEvent(m, *anchor, cluster, supportingIDs)
		if !ok {
			continue
		}
		events = append(events, ev)
	}
	return events, nil
}

// buildEvent maps one plan-derived RawSignal to the Event it should
// become. Returns ok=false for signals that aren't worth persisting
// (e.g. uninteresting risks; we keep risks but mark them low-priority
// in the synthesized stream).
func buildEvent(s, anchor kleio.RawSignal, cluster kleio.Cluster, supporting []string) (kleio.Event, bool) {
	signalType := mapSignalType(s)
	if signalType == "" {
		return kleio.Event{}, false
	}

	created := s.Timestamp
	if created.IsZero() {
		created = time.Now().UTC()
	}

	sd := map[string]any{
		kleio.StructuredKeyClusterAnchorID: cluster.AnchorID,
		kleio.StructuredKeyParentSignalID:  anchor.SourceID,
		kleio.StructuredKeyProvenance:      "plan_cluster",
		"plan_anchor":                      anchorPlanFile(anchor),
		"source_offset":                    s.SourceOffset,
		"is_anchor":                        s.SourceID == cluster.AnchorID,
	}
	if v, ok := s.Metadata["status"]; ok {
		sd["plan_status"] = v
	}
	if v, ok := s.Metadata["deferred"]; ok {
		sd["deferred"] = v
	}
	if v, ok := s.Metadata["rationale"]; ok && v != "" {
		sd["rationale"] = v
	}
	if len(supporting) > 0 {
		sd["supporting_signals"] = supporting
	}
	sdBytes, _ := json.Marshal(sd)

	authorType := "agent"
	if s.Author != "" {
		authorType = "user"
	}

	return kleio.Event{
		ID:             eventIDFor(s),
		SignalType:     signalType,
		Content:        truncateContent(s.Content, 1500),
		SourceType:     s.SourceType,
		CreatedAt:      created.UTC().Format(time.RFC3339),
		RepoName:       s.RepoName,
		FilePath:       anchorPlanFile(anchor),
		StructuredData: string(sdBytes),
		AuthorType:     authorType,
	}, true
}

// mapSignalType decides which signal_type the event takes based on the
// plan signal's SourceOffset prefix. The PlanIngester emits stable
// prefixes ("todo:", "decision:", "deferred:", "risk:", "umbrella")
// so this mapping is deterministic.
func mapSignalType(s kleio.RawSignal) string {
	switch {
	case s.SourceOffset == "umbrella":
		return kleio.SignalTypeCheckpoint
	case strings.HasPrefix(s.SourceOffset, "decision:"):
		return kleio.SignalTypeDecision
	case strings.HasPrefix(s.SourceOffset, "deferred:"):
		return kleio.SignalTypeWorkItem
	case strings.HasPrefix(s.SourceOffset, "todo:"):
		return kleio.SignalTypeWorkItem
	case strings.HasPrefix(s.SourceOffset, "risk:"):
		return kleio.SignalTypeWorkItem
	}
	return ""
}

func anchorPlanFile(anchor kleio.RawSignal) string {
	if v, ok := anchor.Metadata["plan_file"].(string); ok && v != "" {
		return v
	}
	return anchor.SourceID
}

func eventIDFor(s kleio.RawSignal) string {
	return fmt.Sprintf("plan:%s#%s", s.SourceID, s.SourceOffset)
}

// collectSupporting returns the SourceIDs of every non-plan, non-anchor
// member that contributes evidence to this cluster (commits and
// transcripts). Persisted under StructuredData["supporting_signals"]
// so downstream commands can hydrate them without rewalking the link
// graph.
func collectSupporting(cluster kleio.Cluster, anchorID string) []string {
	seen := map[string]bool{anchorID: true}
	var out []string
	for _, m := range cluster.Members {
		if m.SourceID == "" || seen[m.SourceID] {
			continue
		}
		if m.SourceType == "cursor_plan" {
			continue
		}
		seen[m.SourceID] = true
		out = append(out, m.SourceID)
	}
	sort.Strings(out)
	return out
}

func truncateContent(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
