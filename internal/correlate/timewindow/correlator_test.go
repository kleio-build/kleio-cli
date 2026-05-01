package timewindow

import (
	"context"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

func sig(id, kind, source, repo string, ts time.Time, offset string) kleio.RawSignal {
	return kleio.RawSignal{
		SourceType:   source,
		SourceID:     id,
		SourceOffset: offset,
		Kind:         kind,
		Timestamp:    ts,
		RepoName:     repo,
	}
}

func TestCorrelator_NameStable(t *testing.T) {
	if got := New(0).Name(); got != "time_window" {
		t.Errorf("Name()=%q want time_window", got)
	}
}

func TestCorrelate_GroupsSignalsInSameWindow(t *testing.T) {
	now := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	signals := []kleio.RawSignal{
		sig("a", "work_item", "cursor_plan", "r1", now, "todo:1"),
		sig("b", "work_item", "cursor_plan", "r1", now.Add(5*time.Minute), "todo:2"),
		sig("c", "work_item", "cursor_plan", "r1", now.Add(20*time.Minute), "todo:3"),
	}
	c := New(15 * time.Minute)
	clusters, err := c.Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("want 1 cluster (b is within 15m of a; c is outside, but only 1 member -> dropped), got %d", len(clusters))
	}
	if len(clusters[0].Members) != 2 {
		t.Errorf("want 2 members, got %d", len(clusters[0].Members))
	}
}

func TestCorrelate_RespectsRepoBoundary(t *testing.T) {
	now := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	signals := []kleio.RawSignal{
		sig("a", "work_item", "cursor_plan", "repoA", now, "todo:1"),
		sig("b", "work_item", "cursor_plan", "repoB", now, "todo:1"),
		sig("c", "work_item", "cursor_plan", "repoA", now.Add(2*time.Minute), "todo:2"),
	}
	c := New(10 * time.Minute)
	clusters, err := c.Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("want 1 cluster (only repoA pair), got %d", len(clusters))
	}
	if clusters[0].Members[0].RepoName != "repoA" {
		t.Errorf("anchor repo=%q want repoA", clusters[0].Members[0].RepoName)
	}
}

func TestCorrelate_PreferUmbrellaAnchor(t *testing.T) {
	now := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	signals := []kleio.RawSignal{
		sig("plan-todo", "work_item", "cursor_plan", "r1", now, "todo:1"),
		sig("plan-umbrella", "checkpoint", "cursor_plan", "r1", now.Add(time.Minute), "umbrella"),
		sig("git-commit", "git_commit", kleio.SourceTypeLocalGit, "r1", now.Add(2*time.Minute), "commit:abc"),
	}
	c := New(10 * time.Minute)
	clusters, err := c.Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("want 1 cluster, got %d", len(clusters))
	}
	if clusters[0].AnchorID != "plan-umbrella" {
		t.Errorf("AnchorID=%q want plan-umbrella (umbrella > plan-todo > git)", clusters[0].AnchorID)
	}
	if clusters[0].AnchorType != "cursor_plan" {
		t.Errorf("AnchorType=%q want cursor_plan", clusters[0].AnchorType)
	}
}

func TestCorrelate_SkipsSignalsWithZeroTimestamp(t *testing.T) {
	signals := []kleio.RawSignal{
		sig("a", "work_item", "cursor_plan", "r1", time.Time{}, "todo:1"),
		sig("b", "work_item", "cursor_plan", "r1", time.Time{}, "todo:2"),
	}
	clusters, err := New(15 * time.Minute).Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Errorf("want 0 clusters (zero timestamps excluded), got %d", len(clusters))
	}
}

func TestCorrelate_DropsSingletonBuckets(t *testing.T) {
	now := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	signals := []kleio.RawSignal{
		sig("a", "work_item", "cursor_plan", "r1", now, "todo:1"),
		sig("b", "work_item", "cursor_plan", "r1", now.Add(60*time.Minute), "todo:2"),
	}
	clusters, err := New(15 * time.Minute).Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Errorf("want 0 clusters (each bucket has 1 member), got %d", len(clusters))
	}
}

func TestCorrelate_LinksAreOriginatedFromAnchor(t *testing.T) {
	now := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	signals := []kleio.RawSignal{
		sig("plan-umbrella", "checkpoint", "cursor_plan", "r1", now, "umbrella"),
		sig("plan-todo", "work_item", "cursor_plan", "r1", now.Add(time.Minute), "todo:1"),
		sig("git-commit", "git_commit", kleio.SourceTypeLocalGit, "r1", now.Add(2*time.Minute), "commit:abc"),
	}
	clusters, err := New(15 * time.Minute).Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 || len(clusters[0].Links) != 2 {
		t.Fatalf("want 1 cluster with 2 links, got %d clusters", len(clusters))
	}
	for _, l := range clusters[0].Links {
		if l.From != "plan-umbrella" {
			t.Errorf("link.From=%q want plan-umbrella", l.From)
		}
		if l.LinkType != kleio.LinkTypeCorrelatedWith {
			t.Errorf("link.LinkType=%q want %q", l.LinkType, kleio.LinkTypeCorrelatedWith)
		}
	}
}

func TestCorrelate_DefaultsApplied(t *testing.T) {
	c := &Correlator{}
	if got := c.Name(); got != "time_window" {
		t.Errorf("Name=%q want time_window", got)
	}
	now := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	signals := []kleio.RawSignal{
		sig("a", "work_item", "cursor_plan", "r1", now, "todo:1"),
		sig("b", "work_item", "cursor_plan", "r1", now.Add(time.Minute), "todo:2"),
	}
	clusters, err := c.Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("want 1 cluster (defaults apply), got %d", len(clusters))
	}
}
