package search

import (
	"context"
	"strings"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

// fakeStore is a deterministic Store stub. Only the Search method is
// actually used by Correlator.
type fakeStore struct {
	results map[string][]kleio.SearchResult
}

func (f *fakeStore) Search(_ context.Context, query string, _ kleio.SearchOpts) ([]kleio.SearchResult, error) {
	for k, v := range f.results {
		if strings.Contains(query, k) {
			return v, nil
		}
	}
	return nil, nil
}
func (f *fakeStore) CreateEvent(context.Context, *kleio.Event) error                   { return nil }
func (f *fakeStore) ListEvents(context.Context, kleio.EventFilter) ([]kleio.Event, error) {
	return nil, nil
}
func (f *fakeStore) GetEvent(context.Context, string) (*kleio.Event, error)             { return nil, nil }
func (f *fakeStore) CreateBacklogItem(context.Context, *kleio.BacklogItem) error        { return nil }
func (f *fakeStore) ListBacklogItems(context.Context, kleio.BacklogFilter) ([]kleio.BacklogItem, error) {
	return nil, nil
}
func (f *fakeStore) GetBacklogItem(context.Context, string) (*kleio.BacklogItem, error) {
	return nil, nil
}
func (f *fakeStore) UpdateBacklogItem(context.Context, string, *kleio.BacklogItem) error {
	return nil
}
func (f *fakeStore) IndexCommits(context.Context, string, []kleio.Commit) error { return nil }
func (f *fakeStore) QueryCommits(context.Context, kleio.CommitFilter) ([]kleio.Commit, error) {
	return nil, nil
}
func (f *fakeStore) CreateLink(context.Context, *kleio.Link) error               { return nil }
func (f *fakeStore) QueryLinks(context.Context, kleio.LinkFilter) ([]kleio.Link, error) {
	return nil, nil
}
func (f *fakeStore) TrackFileChange(context.Context, *kleio.FileChange) error { return nil }
func (f *fakeStore) FileHistory(context.Context, string) ([]kleio.FileChange, error) {
	return nil, nil
}
func (f *fakeStore) Mode() kleio.StoreMode { return kleio.StoreModeLocal }
func (f *fakeStore) Close() error          { return nil }

func TestCorrelator_Name(t *testing.T) {
	if got := New(nil).Name(); got != "search" {
		t.Errorf("Name=%q want search", got)
	}
}

func TestCorrelator_NilStoreReturnsNoClusters(t *testing.T) {
	c := New(nil)
	clusters, err := c.Correlate(context.Background(), []kleio.RawSignal{
		{SourceID: "s1", Content: "anything"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Errorf("want 0 clusters, got %d", len(clusters))
	}
}

func TestCorrelator_GroupsViaSearchHits(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		{SourceID: "plan1", SourceType: "cursor_plan", Content: "Implement authentication module", Timestamp: now},
		{SourceID: "transcript1", SourceType: kleio.SourceTypeCursorTranscript, Content: "Discussed authentication", Timestamp: now.Add(time.Hour)},
		{SourceID: "commit1", SourceType: kleio.SourceTypeLocalGit, Content: "feat: add auth", Timestamp: now.Add(2 * time.Hour)},
	}
	store := &fakeStore{
		results: map[string][]kleio.SearchResult{
			"authentication": {
				{ID: "transcript1", Score: 0.8, Content: "..."},
				{ID: "commit1", Score: 0.6, Content: "..."},
			},
		},
	}
	c := New(store)
	clusters, err := c.Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("want 1 cluster (3 connected via plan1), got %d: %#v", len(clusters), clusters)
	}
	if clusters[0].AnchorID != "plan1" {
		t.Errorf("AnchorID=%q want plan1", clusters[0].AnchorID)
	}
	if len(clusters[0].Members) != 3 {
		t.Errorf("members=%d want 3", len(clusters[0].Members))
	}
}

func TestCorrelator_LowScoreFiltered(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		{SourceID: "s1", Content: "barely related", Timestamp: now},
		{SourceID: "s2", Content: "barely related", Timestamp: now},
	}
	store := &fakeStore{
		results: map[string][]kleio.SearchResult{
			"barely": {{ID: "s2", Score: 0.05}},
		},
	}
	c := New(store)
	c.MinScore = 0.5
	clusters, err := c.Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Errorf("want 0 clusters (score below threshold), got %d", len(clusters))
	}
}

func TestCorrelator_IgnoresSelfHits(t *testing.T) {
	now := time.Now()
	signals := []kleio.RawSignal{
		{SourceID: "s1", Content: "looks at itself", Timestamp: now},
	}
	store := &fakeStore{
		results: map[string][]kleio.SearchResult{
			"looks": {{ID: "s1", Score: 0.9}},
		},
	}
	clusters, err := New(store).Correlate(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Errorf("want 0 clusters (self-hit + singleton), got %d", len(clusters))
	}
}

func TestBuildQuery_StripsPunctuation(t *testing.T) {
	q := buildQuery("Hello, world! How are you?", 5)
	if strings.ContainsAny(q, ",!?'\"()[]") {
		t.Errorf("query=%q contains punctuation", q)
	}
}

func TestBuildQuery_RespectsTokenLimit(t *testing.T) {
	q := buildQuery("one two three four five six seven eight", 3)
	if got := strings.Count(q, " ") + 1; got != 3 {
		t.Errorf("token count=%d want 3 (q=%q)", got, q)
	}
}
