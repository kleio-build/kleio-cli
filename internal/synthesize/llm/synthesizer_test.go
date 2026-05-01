package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

type fakeProvider struct {
	available bool
	response  string
}

func (f *fakeProvider) Available() bool { return f.available }
func (f *fakeProvider) Complete(_ context.Context, _ string) (string, error) {
	return f.response, nil
}
func (f *fakeProvider) Embed(_ context.Context, _ string) ([]float64, error) {
	return nil, nil
}

func TestSynthesizer_Name(t *testing.T) {
	if got := New(nil).Name(); got != "llm" {
		t.Errorf("Name=%q want llm", got)
	}
}

func TestSynthesizer_NoOpWhenUnavailable(t *testing.T) {
	cluster := kleio.Cluster{
		AnchorID:   "a",
		AnchorType: "cursor_plan",
		Members:    []kleio.RawSignal{{SourceID: "a", Content: "x"}},
	}
	events, err := New(&fakeProvider{available: false}).Synthesize(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("want 0 events when LLM unavailable, got %d", len(events))
	}
}

func TestSynthesizer_SkipsHighConfidenceClusters(t *testing.T) {
	cluster := kleio.Cluster{
		AnchorID:   "a",
		AnchorType: "cursor_plan",
		Confidence: 0.8,
		Members:    []kleio.RawSignal{{SourceID: "a", Content: "should be skipped", Timestamp: time.Now()}},
	}
	provider := &fakeProvider{available: true, response: "SUBJECT: x\nNARRATIVE: y"}
	events, err := New(provider).Synthesize(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("want 0 events for high-confidence cluster (0.8 >= 0.7), got %d", len(events))
	}
}

func TestSynthesizer_RunsForLowConfidenceClusters(t *testing.T) {
	cluster := kleio.Cluster{
		AnchorID:   "a",
		AnchorType: "cursor_plan",
		Confidence: 0.5,
		Members:    []kleio.RawSignal{{SourceID: "a", Content: "should run", Timestamp: time.Now()}},
	}
	provider := &fakeProvider{available: true, response: "SUBJECT: refined\nNARRATIVE: works"}
	events, err := New(provider).Synthesize(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Errorf("want 1 event for low-confidence cluster (0.5 < 0.7), got %d", len(events))
	}
}

func TestSynthesizer_NoOpForEmptyCluster(t *testing.T) {
	cluster := kleio.Cluster{AnchorID: "a", AnchorType: "cursor_plan"}
	provider := &fakeProvider{available: true, response: "SUBJECT: x\nNARRATIVE: y"}
	events, err := New(provider).Synthesize(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("want 0 events for empty cluster, got %d", len(events))
	}
}

func TestSynthesizer_EmitsSummaryEvent(t *testing.T) {
	now := time.Now()
	cluster := kleio.Cluster{
		AnchorID:   "plan:p1",
		AnchorType: "cursor_plan",
		Members: []kleio.RawSignal{
			{SourceID: "plan:p1", SourceType: "cursor_plan", Content: "Pipeline arch", Timestamp: now, RepoName: "kleio-cli"},
		},
	}
	provider := &fakeProvider{
		available: true,
		response:  "SUBJECT: Build pipeline architecture\nNARRATIVE: Implemented Ingest -> Correlate -> Synthesize stages.",
	}
	events, _ := New(provider).Synthesize(context.Background(), cluster)
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].SignalType != kleio.SignalTypeCheckpoint {
		t.Errorf("SignalType=%q want checkpoint", events[0].SignalType)
	}
	if !strings.Contains(events[0].Content, "Build pipeline architecture") {
		t.Errorf("Content=%q does not contain subject", events[0].Content)
	}
	if events[0].SourceType != "llm_summary" {
		t.Errorf("SourceType=%q want llm_summary", events[0].SourceType)
	}
	var sd map[string]any
	_ = json.Unmarshal([]byte(events[0].StructuredData), &sd)
	if sd["subject"] != "Build pipeline architecture" {
		t.Errorf("subject=%v want %q", sd["subject"], "Build pipeline architecture")
	}
}

func TestSynthesizer_HandlesPartialResponse(t *testing.T) {
	now := time.Now()
	cluster := kleio.Cluster{
		AnchorID:   "a",
		AnchorType: "cursor_plan",
		Members:    []kleio.RawSignal{{SourceID: "a", Timestamp: now}},
	}
	provider := &fakeProvider{available: true, response: "SUBJECT: subject only"}
	events, _ := New(provider).Synthesize(context.Background(), cluster)
	if len(events) != 1 {
		t.Fatalf("want 1 event with subject only, got %d", len(events))
	}
	if events[0].Content != "subject only" {
		t.Errorf("Content=%q want %q", events[0].Content, "subject only")
	}
}

func TestSynthesizer_DropsEmptyResponse(t *testing.T) {
	cluster := kleio.Cluster{
		AnchorID:   "a",
		AnchorType: "cursor_plan",
		Members:    []kleio.RawSignal{{SourceID: "a", Timestamp: time.Now()}},
	}
	provider := &fakeProvider{available: true, response: "garbage with no subject or narrative"}
	events, _ := New(provider).Synthesize(context.Background(), cluster)
	if len(events) != 0 {
		t.Errorf("want 0 events when response unparseable, got %d", len(events))
	}
}

func TestParseLLMResponse(t *testing.T) {
	subject, narrative := parseLLMResponse("SUBJECT: foo\nNARRATIVE: bar baz")
	if subject != "foo" {
		t.Errorf("subject=%q want foo", subject)
	}
	if narrative != "bar baz" {
		t.Errorf("narrative=%q want %q", narrative, "bar baz")
	}
}

func TestBuildPrompt_TruncatesMembers(t *testing.T) {
	cluster := kleio.Cluster{
		AnchorID:   "a",
		AnchorType: "cursor_plan",
	}
	for i := 0; i < 50; i++ {
		cluster.Members = append(cluster.Members, kleio.RawSignal{
			SourceID: "x", SourceType: "cursor_plan", SourceOffset: "todo:1",
			Content: "lorem ipsum",
		})
	}
	prompt := buildPrompt(cluster, 10, 200)
	if !strings.Contains(prompt, "40 more members truncated") {
		t.Errorf("prompt did not include truncation hint: %q", prompt)
	}
}
