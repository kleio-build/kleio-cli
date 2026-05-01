package idreference

import (
	"context"
	"strings"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

func mkSig(id, source, content string, ts time.Time, md map[string]any) kleio.RawSignal {
	return kleio.RawSignal{
		SourceID:   id,
		SourceType: source,
		Content:    content,
		Timestamp:  ts,
		Metadata:   md,
	}
}

func TestCorrelator_Name(t *testing.T) {
	if got := New().Name(); got != "id_reference" {
		t.Errorf("Name=%q want id_reference", got)
	}
}

func TestCorrelate_GroupsByKLRef(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		mkSig("s1", "cursor_plan", "Implement KL-12 backlog item", now, nil),
		mkSig("s2", kleio.SourceTypeLocalGit, "feat: address KL-12", now.Add(time.Hour), nil),
		mkSig("s3", "cursor_plan", "Unrelated work", now, nil),
	}
	clusters, err := New().Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("want 1 cluster (KL-12), got %d", len(clusters))
	}
	if len(clusters[0].Members) != 2 {
		t.Errorf("want 2 members, got %d", len(clusters[0].Members))
	}
	if !strings.Contains(clusters[0].Provenance[0], "KL-12") {
		t.Errorf("provenance=%v want KL-12 mention", clusters[0].Provenance)
	}
}

func TestCorrelate_GroupsByPlanFilename(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		mkSig("plan1", "cursor_plan", "Plan body", now, map[string]any{
			"plan_file": "report_quality_fixes_de202626.plan.md",
		}),
		mkSig("commit1", kleio.SourceTypeLocalGit, "wip: report_quality_fixes_de202626", now.Add(time.Hour), nil),
		mkSig("transcript1", kleio.SourceTypeCursorTranscript, "Working on report_quality_fixes_de202626", now.Add(2*time.Hour), nil),
	}
	clusters, err := New().Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	var planCluster *kleio.Cluster
	for i, c := range clusters {
		for _, p := range c.Provenance {
			if strings.Contains(p, "report_quality_fixes_de202626") {
				planCluster = &clusters[i]
				break
			}
		}
	}
	if planCluster == nil {
		t.Fatalf("want plan-id cluster, got %d clusters: %#v", len(clusters), clusters)
	}
	if len(planCluster.Members) != 3 {
		t.Errorf("want 3 members in plan cluster, got %d", len(planCluster.Members))
	}
	if planCluster.AnchorID != "plan1" {
		t.Errorf("AnchorID=%q want plan1 (plan should anchor)", planCluster.AnchorID)
	}
}

func TestCorrelate_GroupsByPRRef(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		mkSig("transcript1", kleio.SourceTypeCursorTranscript, "Discussed merging in PR #42", now, nil),
		mkSig("commit1", kleio.SourceTypeLocalGit, "Merge PR #42 from feat/auth", now.Add(time.Hour), nil),
	}
	clusters, err := New().Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("want 1 cluster (#42), got %d", len(clusters))
	}
	if !strings.Contains(clusters[0].Provenance[0], "#42") {
		t.Errorf("provenance=%v want #42 mention", clusters[0].Provenance)
	}
}

func TestCorrelate_DropsSingletonRefs(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		mkSig("s1", "cursor_plan", "Implement KL-99", now, nil),
		mkSig("s2", "cursor_plan", "Unrelated", now, nil),
	}
	clusters, err := New().Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Errorf("want 0 clusters (KL-99 only mentioned once), got %d", len(clusters))
	}
}

func TestCorrelate_CommitDoesNotSelfClusterOnSHA(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		mkSig("git:r:abcdef1234", kleio.SourceTypeLocalGit, "abcdef1234 fix bug", now, map[string]any{"sha": "abcdef1234"}),
	}
	clusters, err := New().Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Errorf("want 0 clusters (single commit can't self-cluster), got %d", len(clusters))
	}
}

func TestCorrelate_CommitClustersWithSHAReference(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		mkSig("git:r:abc1234567", kleio.SourceTypeLocalGit, "fix bug", now, map[string]any{"sha": "abc1234567"}),
		mkSig("transcript1", kleio.SourceTypeCursorTranscript, "Reviewed commit abc1234", now.Add(time.Hour), nil),
	}
	clusters, err := New().Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("want 1 cluster (sha cross-link), got %d", len(clusters))
	}
	if clusters[0].AnchorID != "git:r:abc1234567" {
		t.Errorf("AnchorID=%q want git anchor", clusters[0].AnchorID)
	}
}

func TestCorrelate_DeterministicOrdering(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		mkSig("s1", "cursor_plan", "KL-1 KL-2", now, nil),
		mkSig("s2", "cursor_plan", "KL-1 KL-2", now.Add(time.Minute), nil),
	}
	a, _ := New().Correlate(context.Background(), signals)
	b, _ := New().Correlate(context.Background(), signals)
	if len(a) != len(b) {
		t.Fatalf("non-deterministic cluster count: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].AnchorID != b[i].AnchorID {
			t.Errorf("non-deterministic anchor at idx=%d: %q vs %q", i, a[i].AnchorID, b[i].AnchorID)
		}
		if len(a[i].Provenance) > 0 && len(b[i].Provenance) > 0 && a[i].Provenance[0] != b[i].Provenance[0] {
			t.Errorf("non-deterministic provenance at idx=%d: %q vs %q", i, a[i].Provenance[0], b[i].Provenance[0])
		}
	}
}
