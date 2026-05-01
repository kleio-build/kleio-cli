package filepath

import (
	"context"
	"strings"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

func sig(id, source, content string, ts time.Time, md map[string]any) kleio.RawSignal {
	return kleio.RawSignal{
		SourceID:   id,
		SourceType: source,
		Content:    content,
		Timestamp:  ts,
		Metadata:   md,
	}
}

func TestCorrelator_Name(t *testing.T) {
	if got := New().Name(); got != "file_path" {
		t.Errorf("Name=%q want file_path", got)
	}
}

func TestCorrelate_GroupsExactPathHits(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		sig("commit1", kleio.SourceTypeLocalGit, "fix bug", now, map[string]any{
			"files": []string{"internal/render/pdf.go"},
		}),
		sig("transcript1", kleio.SourceTypeCursorTranscript, "Edited internal/render/pdf.go", now.Add(time.Hour), nil),
		sig("commit2", kleio.SourceTypeLocalGit, "unrelated", now, map[string]any{
			"files": []string{"internal/other/foo.go"},
		}),
	}
	clusters, err := New().Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	var pdfCluster *kleio.Cluster
	for i, c := range clusters {
		for _, p := range c.Provenance {
			if strings.Contains(p, "internal/render/pdf.go") {
				pdfCluster = &clusters[i]
				break
			}
		}
	}
	if pdfCluster == nil {
		t.Fatalf("want exact-path cluster for pdf.go, got %#v", clusters)
	}
	if len(pdfCluster.Members) != 2 {
		t.Errorf("want 2 members in pdf cluster, got %d", len(pdfCluster.Members))
	}
}

func TestCorrelate_NormalisesWindowsPaths(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		sig("commit1", kleio.SourceTypeLocalGit, "x", now, map[string]any{
			"files": []string{"internal\\render\\pdf.go"},
		}),
		sig("commit2", kleio.SourceTypeLocalGit, "y", now, map[string]any{
			"files": []string{"internal/render/pdf.go"},
		}),
	}
	clusters, err := New().Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) == 0 {
		t.Fatalf("want cluster despite separator differences, got 0")
	}
}

func TestCorrelate_SharedDirEmitsLowerConfidence(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		sig("commit1", kleio.SourceTypeLocalGit, "x", now, map[string]any{
			"files": []string{"internal/ingest/plan/parser.go"},
		}),
		sig("commit2", kleio.SourceTypeLocalGit, "y", now, map[string]any{
			"files": []string{"internal/ingest/plan/ingester.go"},
		}),
	}
	clusters, err := New().Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) == 0 {
		t.Fatalf("want shared-dir cluster, got 0")
	}
	for _, c := range clusters {
		if c.Confidence < 0.4 || c.Confidence > 0.5 {
			t.Errorf("shared-dir cluster confidence=%.2f want ~0.45", c.Confidence)
		}
	}
}

func TestCorrelate_DropsSingletonPaths(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		sig("commit1", kleio.SourceTypeLocalGit, "x", now, map[string]any{
			"files": []string{"internal/foo.go"},
		}),
	}
	clusters, err := New().Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Errorf("want 0 clusters (singleton path), got %d", len(clusters))
	}
}

func TestCorrelate_PullsInlinePathsFromContent(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		sig("plan1", "cursor_plan", "Touch internal/render/pdf.go", now, nil),
		sig("commit1", kleio.SourceTypeLocalGit, "fix render", now.Add(time.Hour), map[string]any{
			"files": []string{"internal/render/pdf.go"},
		}),
	}
	clusters, err := New().Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) == 0 {
		t.Fatalf("want cluster from inline-path extraction, got 0")
	}
	if clusters[0].AnchorID != "plan1" {
		t.Errorf("AnchorID=%q want plan1 (plans are preferred anchors)", clusters[0].AnchorID)
	}
}

func TestCorrelate_NoExactPathPreventsDirCluster(t *testing.T) {
	now := time.Now()
	// Two signals referencing the same exact path should ONLY produce
	// the exact cluster (the dir cluster carries the same members and
	// should be deduped).
	signals := []kleio.RawSignal{
		sig("a", kleio.SourceTypeLocalGit, "x", now, map[string]any{"files": []string{"internal/x/y.go"}}),
		sig("b", kleio.SourceTypeLocalGit, "y", now, map[string]any{"files": []string{"internal/x/y.go"}}),
	}
	clusters, err := New().Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Errorf("want 1 cluster (dir dedup), got %d: %#v", len(clusters), clusters)
	}
}

func TestDirOf_RespectsDepth(t *testing.T) {
	cases := map[string]string{
		"internal/ingest/plan/parser.go": "internal/ingest/plan",
		"a/b/c.go":                       "a/b/", // 2 segments only -> "" since depth=3
		"internal/foo.go":                "",
	}
	cases["a/b/c.go"] = "" // adjusted: dir is "a/b", 2 segments < depth 3 -> ""
	for in, want := range cases {
		got := dirOf(normalizePath(in), 3)
		if got != want {
			t.Errorf("dirOf(%q)=%q want %q", in, got, want)
		}
	}
}
