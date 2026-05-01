package embed

import (
	"context"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

// fakeProvider implements ai.Provider with deterministic embeddings:
// it returns a fixed-length vector derived from the SHA-like prefix of
// the input. Two inputs with the same prefix produce identical vectors
// (cosine = 1); different prefixes produce orthogonal-ish vectors.
type fakeProvider struct {
	available bool
	embeds    map[string][]float64
}

func (f *fakeProvider) Available() bool { return f.available }
func (f *fakeProvider) Complete(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (f *fakeProvider) Embed(_ context.Context, text string) ([]float64, error) {
	if v, ok := f.embeds[text]; ok {
		return v, nil
	}
	return []float64{0, 0, 0}, nil
}

func TestCorrelator_Name(t *testing.T) {
	if got := New(nil).Name(); got != "embed" {
		t.Errorf("Name=%q want embed", got)
	}
}

func TestCorrelator_AvailableFalseWhenNoProvider(t *testing.T) {
	c := New(nil)
	if c.Available() {
		t.Error("Available()=true want false")
	}
}

func TestCorrelator_NoOpWhenUnavailable(t *testing.T) {
	c := New(&fakeProvider{available: false})
	clusters, err := c.Correlate(context.Background(), []kleio.RawSignal{
		{SourceID: "a", Content: "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Errorf("want 0 clusters when unavailable, got %d", len(clusters))
	}
}

func TestCorrelator_GroupsHighCosineSignals(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		{SourceID: "plan1", SourceType: "cursor_plan", Content: "auth", Timestamp: now},
		{SourceID: "transcript1", SourceType: kleio.SourceTypeCursorTranscript, Content: "auth", Timestamp: now.Add(time.Hour)},
		{SourceID: "unrelated1", SourceType: kleio.SourceTypeCursorTranscript, Content: "deploy", Timestamp: now.Add(2 * time.Hour)},
	}
	provider := &fakeProvider{
		available: true,
		embeds: map[string][]float64{
			"auth":   {1, 0, 0},
			"deploy": {0, 1, 0},
		},
	}
	c := New(provider)
	c.Threshold = 0.9
	clusters, err := c.Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("want 1 cluster (auth pair), got %d: %#v", len(clusters), clusters)
	}
	if clusters[0].AnchorID != "plan1" {
		t.Errorf("AnchorID=%q want plan1", clusters[0].AnchorID)
	}
	if len(clusters[0].Members) != 2 {
		t.Errorf("members=%d want 2", len(clusters[0].Members))
	}
}

func TestCorrelator_RespectsRepoBoundary(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		{SourceID: "a", Content: "x", RepoName: "repoA", Timestamp: now},
		{SourceID: "b", Content: "x", RepoName: "repoB", Timestamp: now},
	}
	provider := &fakeProvider{
		available: true,
		embeds:    map[string][]float64{"x": {1, 0, 0}},
	}
	clusters, err := New(provider).Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Errorf("want 0 clusters (cross-repo blocked), got %d", len(clusters))
	}
}

func TestCorrelator_ThresholdFiltersDissimilar(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		{SourceID: "a", Content: "x", Timestamp: now},
		{SourceID: "b", Content: "y", Timestamp: now},
	}
	provider := &fakeProvider{
		available: true,
		embeds: map[string][]float64{
			"x": {1, 0, 0},
			"y": {0.5, 0.5, 0.7}, // cosine ~0.5
		},
	}
	c := New(provider)
	c.Threshold = 0.9
	clusters, err := c.Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Errorf("want 0 clusters (below threshold), got %d", len(clusters))
	}
}

func TestCosineHandlesEdgeCases(t *testing.T) {
	if _, ok := cosine([]float64{0, 0, 0}, []float64{1, 1, 1}); ok {
		t.Error("zero vector cosine should return ok=false")
	}
	if _, ok := cosine([]float64{1, 0}, []float64{1, 0, 0}); ok {
		t.Error("dimension mismatch should return ok=false")
	}
	sim, _ := cosine([]float64{1, 0}, []float64{1, 0})
	if sim < 0.99 {
		t.Errorf("identical vectors cosine=%f want ~1.0", sim)
	}
	sim, _ = cosine([]float64{1, 0}, []float64{0, 1})
	if sim > 0.01 {
		t.Errorf("orthogonal vectors cosine=%f want ~0.0", sim)
	}
}

func TestTruncateRespectsLimit(t *testing.T) {
	s := "abcdefgh"
	if got := truncate(s, 5); got != "abcde" {
		t.Errorf("truncate=%q want abcde", got)
	}
	if got := truncate(s, 100); got != s {
		t.Errorf("truncate(no-op)=%q want %q", got, s)
	}
}
