package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/kleio-build/kleio-cli/internal/ingest/discovery"
	kleio "github.com/kleio-build/kleio-core"
)

type stubProvider struct{ available bool }

func (s *stubProvider) Available() bool                                      { return s.available }
func (s *stubProvider) Complete(_ context.Context, _ string) (string, error) { return "", nil }
func (s *stubProvider) Embed(_ context.Context, _ string) ([]float64, error) { return nil, nil }

type stubStore struct{}

func (stubStore) CreateEvent(context.Context, *kleio.Event) error            { return nil }
func (stubStore) ListEvents(context.Context, kleio.EventFilter) ([]kleio.Event, error) {
	return nil, nil
}
func (stubStore) GetEvent(context.Context, string) (*kleio.Event, error)        { return nil, nil }
func (stubStore) CreateBacklogItem(context.Context, *kleio.BacklogItem) error   { return nil }
func (stubStore) ListBacklogItems(context.Context, kleio.BacklogFilter) ([]kleio.BacklogItem, error) {
	return nil, nil
}
func (stubStore) GetBacklogItem(context.Context, string) (*kleio.BacklogItem, error) {
	return nil, nil
}
func (stubStore) UpdateBacklogItem(context.Context, string, *kleio.BacklogItem) error {
	return nil
}
func (stubStore) IndexCommits(context.Context, string, []kleio.Commit) error { return nil }
func (stubStore) QueryCommits(context.Context, kleio.CommitFilter) ([]kleio.Commit, error) {
	return nil, nil
}
func (stubStore) CreateLink(context.Context, *kleio.Link) error             { return nil }
func (stubStore) QueryLinks(context.Context, kleio.LinkFilter) ([]kleio.Link, error) {
	return nil, nil
}
func (stubStore) TrackFileChange(context.Context, *kleio.FileChange) error { return nil }
func (stubStore) FileHistory(context.Context, string) ([]kleio.FileChange, error) {
	return nil, nil
}
func (stubStore) Search(context.Context, string, kleio.SearchOpts) ([]kleio.SearchResult, error) {
	return nil, nil
}
func (stubStore) CreateEntity(context.Context, *kleio.Entity) error   { return nil }
func (stubStore) FindEntity(context.Context, string, string) (*kleio.Entity, error) {
	return nil, nil
}
func (stubStore) ListEntities(context.Context, kleio.EntityFilter) ([]kleio.Entity, error) {
	return nil, nil
}
func (stubStore) CreateEntityAlias(context.Context, *kleio.EntityAlias) error   { return nil }
func (stubStore) CreateEntityMention(context.Context, *kleio.EntityMention) error { return nil }
func (stubStore) FindEntitiesByEvidence(context.Context, string) ([]kleio.Entity, error) {
	return nil, nil
}
func (stubStore) Mode() kleio.StoreMode { return kleio.StoreModeLocal }
func (stubStore) Close() error          { return nil }

func TestBuild_DefaultsApplied(t *testing.T) {
	cfg := Config{
		Discovery: discovery.Resolve(t.TempDir(), false),
		Store:     stubStore{},
	}
	p := Build(cfg)
	if p == nil {
		t.Fatal("nil pipeline")
	}
	if len(p.Ingesters) != 3 {
		t.Errorf("ingesters=%d want 3 (plan+transcript+git)", len(p.Ingesters))
	}
	if len(p.Correlators) != 5 {
		t.Errorf("correlators=%d want 5 (time+id+filepath+entity_overlap+search)", len(p.Correlators))
	}
	if len(p.Synthesizers) != 2 {
		t.Errorf("synthesizers=%d want 2 (plan+orphan; no LLM)", len(p.Synthesizers))
	}
}

func TestBuild_LLMPromotesEmbedAndLLMSynth(t *testing.T) {
	cfg := Config{
		Discovery: discovery.Resolve(t.TempDir(), false),
		Store:     stubStore{},
		Provider:  &stubProvider{available: true},
	}
	p := Build(cfg)
	if len(p.Synthesizers) != 3 {
		t.Errorf("synthesizers=%d want 3 (plan+orphan+llm)", len(p.Synthesizers))
	}
	hasEmbed := false
	for _, c := range p.Correlators {
		if c.Name() == "embed" {
			hasEmbed = true
		}
	}
	if !hasEmbed {
		t.Error("embed correlator not present when LLM available")
	}
	hasSearch := false
	for _, c := range p.Correlators {
		if c.Name() == "search" {
			hasSearch = true
		}
	}
	if hasSearch {
		t.Error("search correlator should be REPLACED by embed when LLM available")
	}
}

func TestBuild_FilterByEnabledIngesters(t *testing.T) {
	cfg := Config{
		Discovery:        discovery.Resolve(t.TempDir(), false),
		Store:            stubStore{},
		EnabledIngesters: map[string]bool{"plan": true},
	}
	p := Build(cfg)
	if len(p.Ingesters) != 1 || p.Ingesters[0].Name() != "plan" {
		t.Errorf("filtered ingesters=%v want [plan]", names(p.Ingesters))
	}
}

func TestBuild_FilterByEnabledSynthesizers(t *testing.T) {
	cfg := Config{
		Discovery:           discovery.Resolve(t.TempDir(), false),
		Store:               stubStore{},
		EnabledSynthesizers: map[string]bool{"orphan": true},
	}
	p := Build(cfg)
	if len(p.Synthesizers) != 1 || p.Synthesizers[0].Name() != "orphan" {
		t.Errorf("filtered synthesizers=%v want [orphan]", synthNames(p.Synthesizers))
	}
}

func TestBuild_RespectsTimeWindow(t *testing.T) {
	cfg := Config{
		Discovery:  discovery.Resolve(t.TempDir(), false),
		Store:      stubStore{},
		TimeWindow: 5 * time.Minute,
	}
	p := Build(cfg)
	if len(p.Correlators) == 0 {
		t.Fatal("no correlators")
	}
	// First correlator should be timewindow with 5min window. We
	// can't introspect the window directly but Name() is enough to
	// confirm it's wired.
	if p.Correlators[0].Name() != "time_window" {
		t.Errorf("first correlator=%q want time_window", p.Correlators[0].Name())
	}
}

func names(in []kleio.Ingester) []string {
	out := make([]string, len(in))
	for i, x := range in {
		out[i] = x.Name()
	}
	return out
}

func synthNames(in []kleio.Synthesizer) []string {
	out := make([]string, len(in))
	for i, x := range in {
		out[i] = x.Name()
	}
	return out
}
