package plancluster

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

func planSig(id, offset, content string, ts time.Time, md map[string]any) kleio.RawSignal {
	return kleio.RawSignal{
		SourceID:     id,
		SourceType:   "cursor_plan",
		SourceOffset: offset,
		Content:      content,
		Timestamp:    ts,
		RepoName:     "kleio-cli",
		Metadata:     md,
	}
}

func gitSig(id, content string, ts time.Time) kleio.RawSignal {
	return kleio.RawSignal{
		SourceID:   id,
		SourceType: kleio.SourceTypeLocalGit,
		Content:    content,
		Timestamp:  ts,
		RepoName:   "kleio-cli",
	}
}

func transcriptSig(id, content string, ts time.Time) kleio.RawSignal {
	return kleio.RawSignal{
		SourceID:   id,
		SourceType: kleio.SourceTypeCursorTranscript,
		Content:    content,
		Timestamp:  ts,
		RepoName:   "kleio-cli",
	}
}

func TestSynthesizer_Name(t *testing.T) {
	if got := New().Name(); got != "plan_cluster" {
		t.Errorf("Name=%q want plan_cluster", got)
	}
}

func TestSynthesizer_SkipsNonPlanAnchoredClusters(t *testing.T) {
	now := time.Now()
	cluster := kleio.Cluster{
		AnchorID:   "git:abc",
		AnchorType: kleio.SourceTypeLocalGit,
		Members:    []kleio.RawSignal{gitSig("git:abc", "x", now)},
	}
	events, err := New().Synthesize(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("want 0 events for non-plan-anchored cluster, got %d", len(events))
	}
}

func TestSynthesizer_EmitsCheckpointForUmbrella(t *testing.T) {
	now := time.Now()
	umbrella := planSig("plan:p1", "umbrella", "Plan summary", now, map[string]any{
		"plan_file": "p1.plan.md",
	})
	cluster := kleio.Cluster{
		AnchorID:   umbrella.SourceID,
		AnchorType: "cursor_plan",
		Members:    []kleio.RawSignal{umbrella},
	}
	events, err := New().Synthesize(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 event (umbrella checkpoint), got %d", len(events))
	}
	if events[0].SignalType != kleio.SignalTypeCheckpoint {
		t.Errorf("SignalType=%q want %q", events[0].SignalType, kleio.SignalTypeCheckpoint)
	}
	var sd map[string]any
	_ = json.Unmarshal([]byte(events[0].StructuredData), &sd)
	if sd[kleio.StructuredKeyClusterAnchorID] != umbrella.SourceID {
		t.Errorf("cluster_anchor_id=%v want %s", sd[kleio.StructuredKeyClusterAnchorID], umbrella.SourceID)
	}
	if sd["is_anchor"] != true {
		t.Errorf("is_anchor=%v want true", sd["is_anchor"])
	}
}

func TestSynthesizer_EmitsWorkItemForTodos(t *testing.T) {
	now := time.Now()
	umbrella := planSig("plan:p1", "umbrella", "Plan", now, nil)
	todo := planSig("plan:p1#todo:t1", "todo:t1", "Implement X", now, map[string]any{
		"status": "pending",
	})
	cluster := kleio.Cluster{
		AnchorID:   umbrella.SourceID,
		AnchorType: "cursor_plan",
		Members:    []kleio.RawSignal{umbrella, todo},
	}
	events, err := New().Synthesize(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("want 2 events (umbrella + todo), got %d", len(events))
	}
	var todoEvent *kleio.Event
	for i, e := range events {
		if strings.Contains(e.ID, "todo:t1") {
			todoEvent = &events[i]
		}
	}
	if todoEvent == nil {
		t.Fatal("todo event missing")
	}
	if todoEvent.SignalType != kleio.SignalTypeWorkItem {
		t.Errorf("todo SignalType=%q want %q", todoEvent.SignalType, kleio.SignalTypeWorkItem)
	}
	var sd map[string]any
	_ = json.Unmarshal([]byte(todoEvent.StructuredData), &sd)
	if sd["plan_status"] != "pending" {
		t.Errorf("plan_status=%v want pending", sd["plan_status"])
	}
	if sd[kleio.StructuredKeyParentSignalID] != umbrella.SourceID {
		t.Errorf("parent_signal_id=%v want %q", sd[kleio.StructuredKeyParentSignalID], umbrella.SourceID)
	}
}

func TestSynthesizer_EmitsDecisionForDecisionBlock(t *testing.T) {
	now := time.Now()
	umbrella := planSig("plan:p1", "umbrella", "Plan", now, nil)
	decision := planSig("plan:p1#decision:0", "decision:0", "Use FTS5", now, map[string]any{
		"rationale": "avoids triggers",
	})
	cluster := kleio.Cluster{
		AnchorID:   umbrella.SourceID,
		AnchorType: "cursor_plan",
		Members:    []kleio.RawSignal{umbrella, decision},
	}
	events, err := New().Synthesize(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	var decisionEvent *kleio.Event
	for i, e := range events {
		if e.SignalType == kleio.SignalTypeDecision {
			decisionEvent = &events[i]
		}
	}
	if decisionEvent == nil {
		t.Fatal("decision event missing")
	}
	var sd map[string]any
	_ = json.Unmarshal([]byte(decisionEvent.StructuredData), &sd)
	if sd["rationale"] != "avoids triggers" {
		t.Errorf("rationale=%v want %q", sd["rationale"], "avoids triggers")
	}
}

func TestSynthesizer_FlagsDeferredItems(t *testing.T) {
	now := time.Now()
	umbrella := planSig("plan:p1", "umbrella", "Plan", now, nil)
	deferred := planSig("plan:p1#deferred:0", "deferred:0", "Defer caching", now, map[string]any{
		"deferred": true,
	})
	cluster := kleio.Cluster{
		AnchorID:   umbrella.SourceID,
		AnchorType: "cursor_plan",
		Members:    []kleio.RawSignal{umbrella, deferred},
	}
	events, _ := New().Synthesize(context.Background(), cluster)
	var deferredEvent *kleio.Event
	for i, e := range events {
		if strings.Contains(e.ID, "deferred:0") {
			deferredEvent = &events[i]
		}
	}
	if deferredEvent == nil {
		t.Fatal("deferred event missing")
	}
	if deferredEvent.SignalType != kleio.SignalTypeWorkItem {
		t.Errorf("deferred event SignalType=%q want work_item", deferredEvent.SignalType)
	}
	var sd map[string]any
	_ = json.Unmarshal([]byte(deferredEvent.StructuredData), &sd)
	if sd["deferred"] != true {
		t.Errorf("deferred=%v want true", sd["deferred"])
	}
}

func TestSynthesizer_AttachesSupportingSignals(t *testing.T) {
	now := time.Now()
	umbrella := planSig("plan:p1", "umbrella", "Plan", now, nil)
	todo := planSig("plan:p1#todo:t1", "todo:t1", "Implement X", now, nil)
	commit := gitSig("git:abc", "feat: x", now.Add(time.Hour))
	transcript := transcriptSig("trans:1", "Working on X", now.Add(2*time.Hour))
	cluster := kleio.Cluster{
		AnchorID:   umbrella.SourceID,
		AnchorType: "cursor_plan",
		Members:    []kleio.RawSignal{umbrella, todo, commit, transcript},
	}
	events, _ := New().Synthesize(context.Background(), cluster)
	if len(events) != 2 {
		t.Fatalf("want 2 events (only plan members promoted), got %d", len(events))
	}
	for _, e := range events {
		var sd map[string]any
		_ = json.Unmarshal([]byte(e.StructuredData), &sd)
		supporting, _ := sd["supporting_signals"].([]any)
		if len(supporting) != 2 {
			t.Errorf("event %s supporting_signals=%v want 2", e.ID, supporting)
		}
	}
}

func TestSynthesizer_SkipsClusterWithMissingAnchor(t *testing.T) {
	now := time.Now()
	todo := planSig("plan:p1#todo:t1", "todo:t1", "Implement X", now, nil)
	cluster := kleio.Cluster{
		AnchorID:   "plan:missing",
		AnchorType: "cursor_plan",
		Members:    []kleio.RawSignal{todo}, // anchor not in members
	}
	events, _ := New().Synthesize(context.Background(), cluster)
	if len(events) != 0 {
		t.Errorf("want 0 events when anchor missing from members, got %d", len(events))
	}
}
