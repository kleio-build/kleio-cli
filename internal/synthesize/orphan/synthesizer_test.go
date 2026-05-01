package orphan

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

func mkCluster(anchorType, anchorID string, members ...kleio.RawSignal) kleio.Cluster {
	return kleio.Cluster{
		AnchorID:   anchorID,
		AnchorType: anchorType,
		Members:    members,
	}
}

func TestSynthesizer_Name(t *testing.T) {
	if got := New().Name(); got != "orphan" {
		t.Errorf("Name=%q want orphan", got)
	}
}

func TestSynthesizer_SkipsPlanAnchoredClusters(t *testing.T) {
	now := time.Now()
	cluster := mkCluster("cursor_plan", "plan:p1",
		kleio.RawSignal{SourceID: "plan:p1", SourceType: "cursor_plan", Timestamp: now},
		kleio.RawSignal{
			SourceID: "trans:1", SourceType: kleio.SourceTypeCursorTranscript,
			SourceOffset: "toolcall:L10", Timestamp: now,
		},
	)
	events, err := New().Synthesize(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("want 0 events for plan-anchored cluster, got %d", len(events))
	}
}

func TestSynthesizer_PromotesToolCallTranscripts(t *testing.T) {
	now := time.Now()
	cluster := mkCluster(kleio.SourceTypeCursorTranscript, "trans:1",
		kleio.RawSignal{
			SourceID:     "trans:1",
			SourceType:   kleio.SourceTypeCursorTranscript,
			SourceOffset: "toolcall:L10",
			Content:      "Use FTS5",
			Kind:         kleio.SignalTypeDecision,
			Timestamp:    now,
		},
	)
	events, _ := New().Synthesize(context.Background(), cluster)
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].SignalType != kleio.SignalTypeDecision {
		t.Errorf("SignalType=%q want decision", events[0].SignalType)
	}
	var sd map[string]any
	_ = json.Unmarshal([]byte(events[0].StructuredData), &sd)
	if sd["channel"] != "kleio_mcp" {
		t.Errorf("channel=%v want kleio_mcp", sd["channel"])
	}
}

func TestSynthesizer_PromotesRetroAsks(t *testing.T) {
	now := time.Now()
	cluster := mkCluster(kleio.SourceTypeCursorTranscript, "trans:1",
		kleio.RawSignal{
			SourceID:     "trans:1#retro_ask:L20",
			SourceType:   kleio.SourceTypeCursorTranscript,
			SourceOffset: "retro_ask:L20",
			Content:      "Actually, also add a --pdf flag.",
			Timestamp:    now,
		},
	)
	events, _ := New().Synthesize(context.Background(), cluster)
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].SignalType != kleio.SignalTypeWorkItem {
		t.Errorf("SignalType=%q want work_item", events[0].SignalType)
	}
	if events[0].AuthorType != "user" {
		t.Errorf("AuthorType=%q want user", events[0].AuthorType)
	}
}

func TestSynthesizer_RejectsDeferralTranscripts(t *testing.T) {
	now := time.Now()
	cluster := mkCluster(kleio.SourceTypeCursorTranscript, "trans:1",
		kleio.RawSignal{
			SourceID:     "trans:1#defer:L30",
			SourceType:   kleio.SourceTypeCursorTranscript,
			SourceOffset: "defer:L30",
			Content:      "I'll defer the slack integration.",
			Timestamp:    now,
		},
	)
	events, _ := New().Synthesize(context.Background(), cluster)
	if len(events) != 0 {
		t.Errorf("want 0 events for orphan defer (only plan deferrals are kept), got %d", len(events))
	}
}

func TestSynthesizer_PromotesConventionalCommits(t *testing.T) {
	now := time.Now()
	cases := []struct {
		msg      string
		expected bool
	}{
		{"feat: add auth", true},
		{"fix(api): null check", true},
		{"perf!: drop legacy code", true},
		{"docs: update readme", true},
		{"WIP something", false},
		{"random commit message", false},
	}
	for _, tc := range cases {
		cluster := mkCluster(kleio.SourceTypeLocalGit, "git:abc",
			kleio.RawSignal{
				SourceID:   "git:abc:" + tc.msg,
				SourceType: kleio.SourceTypeLocalGit,
				Content:    tc.msg,
				Timestamp:  now,
				Metadata:   map[string]any{"branch": "main"},
			},
		)
		events, _ := New().Synthesize(context.Background(), cluster)
		if tc.expected && len(events) != 1 {
			t.Errorf("commit %q: want 1 event, got %d", tc.msg, len(events))
		}
		if !tc.expected && len(events) != 0 {
			t.Errorf("commit %q: want 0 events, got %d", tc.msg, len(events))
		}
	}
}

func TestSynthesizer_AnnotatesClusterAnchorID(t *testing.T) {
	now := time.Now()
	cluster := mkCluster(kleio.SourceTypeCursorTranscript, "trans:anchor",
		kleio.RawSignal{
			SourceID: "trans:anchor", SourceType: kleio.SourceTypeCursorTranscript,
			SourceOffset: "toolcall:L1", Content: "x", Kind: kleio.SignalTypeWorkItem,
			Timestamp: now,
		},
	)
	events, _ := New().Synthesize(context.Background(), cluster)
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	var sd map[string]any
	_ = json.Unmarshal([]byte(events[0].StructuredData), &sd)
	if sd[kleio.StructuredKeyClusterAnchorID] != "trans:anchor" {
		t.Errorf("cluster_anchor_id=%v want trans:anchor", sd[kleio.StructuredKeyClusterAnchorID])
	}
	if sd[kleio.StructuredKeyProvenance] != "orphan" {
		t.Errorf("provenance=%v want orphan", sd[kleio.StructuredKeyProvenance])
	}
}
