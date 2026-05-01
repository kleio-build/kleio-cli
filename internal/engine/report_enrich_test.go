package engine

import (
	"context"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	completeResp string
	embeddings   map[string][]float64
}

func (m *mockProvider) Available() bool { return true }
func (m *mockProvider) Complete(_ context.Context, _ string) (string, error) {
	return m.completeResp, nil
}
func (m *mockProvider) Embed(_ context.Context, text string) ([]float64, error) {
	if v, ok := m.embeddings[text]; ok {
		return v, nil
	}
	return []float64{0.1, 0.2, 0.3}, nil
}

func TestEnrich_SubjectRewrite(t *testing.T) {
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	store := localdb.New(db)
	t.Cleanup(func() { store.Close() })

	eng := New(store, nil)
	now := time.Now()

	entries := []TimelineEntry{
		{Timestamp: now, Kind: kleio.SignalTypeGitCommit, Summary: "add auth", SHA: "abc"},
	}

	r := eng.BuildReport(context.Background(), "auth", "trace", entries)
	origSubject := r.Subject

	provider := &mockProvider{completeResp: "A concise narrative about the auth changes."}
	err = r.Enrich(context.Background(), provider)
	require.NoError(t, err)

	assert.True(t, r.Enriched)
	assert.NotEqual(t, origSubject, r.Subject)
	assert.Equal(t, "A concise narrative about the auth changes.", r.Subject)
}

func TestEnrich_SemanticDedup(t *testing.T) {
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	store := localdb.New(db)
	t.Cleanup(func() { store.Close() })

	eng := New(store, nil)
	now := time.Now()

	entries := []TimelineEntry{
		{Timestamp: now.Add(-2 * time.Hour), Kind: kleio.SignalTypeWorkItem, Summary: "Add error handling to payments"},
		{Timestamp: now.Add(-1 * time.Hour), Kind: kleio.SignalTypeWorkItem, Summary: "Implement error handling for payment flow"},
		{Timestamp: now, Kind: kleio.SignalTypeWorkItem, Summary: "Completely different: setup CI"},
	}

	r := eng.BuildReport(context.Background(), "payments", "trace", entries)
	require.Len(t, r.OpenThreads, 3)

	provider := &mockProvider{
		completeResp: "kleio trace payments",
		embeddings: map[string][]float64{
			"Add error handling to payments":           {0.9, 0.1, 0.0},
			"Implement error handling for payment flow": {0.88, 0.12, 0.01},
			"Completely different: setup CI":            {0.0, 0.0, 1.0},
		},
	}

	err = r.Enrich(context.Background(), provider)
	require.NoError(t, err)

	assert.Len(t, r.OpenThreads, 2, "similar threads should be merged")
	assert.True(t, r.Enriched)
}

func TestEnrich_NextSteps(t *testing.T) {
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	store := localdb.New(db)
	t.Cleanup(func() { store.Close() })

	eng := New(store, nil)
	entries := []TimelineEntry{
		{Timestamp: time.Now(), Kind: kleio.SignalTypeGitCommit, Summary: "init", SHA: "abc"},
	}

	r := eng.BuildReport(context.Background(), "auth", "trace", entries)
	origSteps := r.NextSteps

	provider := &mockProvider{
		completeResp: "kleio trace auth --since 30d\nkleio explain HEAD~5 HEAD\nkleio backlog list --search auth",
	}

	err = r.Enrich(context.Background(), provider)
	require.NoError(t, err)

	assert.NotEqual(t, origSteps, r.NextSteps)
	assert.Len(t, r.NextSteps, 3)
}

func TestEnrich_NoopProvider(t *testing.T) {
	db, err := localdb.OpenInMemory()
	require.NoError(t, err)
	store := localdb.New(db)
	t.Cleanup(func() { store.Close() })

	eng := New(store, nil)
	r := eng.BuildReport(context.Background(), "test", "trace", nil)

	noopProv := &noopProv{}
	err = r.Enrich(context.Background(), noopProv)
	require.NoError(t, err)
	assert.False(t, r.Enriched, "Noop provider should not set Enriched")
}

type noopProv struct{}

func (n *noopProv) Available() bool                                        { return false }
func (n *noopProv) Complete(_ context.Context, _ string) (string, error)   { return "", nil }
func (n *noopProv) Embed(_ context.Context, _ string) ([]float64, error) { return nil, nil }
