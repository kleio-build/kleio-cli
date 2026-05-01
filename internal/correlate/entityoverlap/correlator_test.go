package entityoverlap

import (
	"context"
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorrelator_Name(t *testing.T) {
	c := New(nil)
	assert.Equal(t, "entity_overlap", c.Name())
}

func TestCorrelator_SharedTicketCreateCluster(t *testing.T) {
	c := New(nil)

	signals := []kleio.RawSignal{
		{
			SourceID: "sig-1",
			Content:  "fix KLEIO-42",
			Metadata: map[string]any{
				"extracted_entities": []entity.ExtractedEntity{
					{Kind: kleio.EntityKindTicket, Value: "KLEIO-42", Confidence: 0.9},
				},
			},
		},
		{
			SourceID: "sig-2",
			Content:  "plan for KLEIO-42",
			Metadata: map[string]any{
				"extracted_entities": []entity.ExtractedEntity{
					{Kind: kleio.EntityKindTicket, Value: "KLEIO-42", Confidence: 0.9},
				},
			},
		},
	}

	clusters, err := c.Correlate(context.Background(), signals)
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	assert.Len(t, clusters[0].Members, 2)
	assert.InDelta(t, 0.85, clusters[0].Confidence, 0.01)
}

func TestCorrelator_NoOverlapNoClusters(t *testing.T) {
	c := New(nil)

	signals := []kleio.RawSignal{
		{
			SourceID: "sig-1",
			Content:  "fix KLEIO-42",
			Metadata: map[string]any{
				"extracted_entities": []entity.ExtractedEntity{
					{Kind: kleio.EntityKindTicket, Value: "KLEIO-42", Confidence: 0.9},
				},
			},
		},
		{
			SourceID: "sig-2",
			Content:  "update PROJ-99",
			Metadata: map[string]any{
				"extracted_entities": []entity.ExtractedEntity{
					{Kind: kleio.EntityKindTicket, Value: "PROJ-99", Confidence: 0.9},
				},
			},
		},
	}

	clusters, err := c.Correlate(context.Background(), signals)
	require.NoError(t, err)
	assert.Empty(t, clusters)
}

func TestCorrelator_SharedFileCreateCluster(t *testing.T) {
	c := New(nil)

	signals := []kleio.RawSignal{
		{
			SourceID: "sig-1",
			Metadata: map[string]any{
				"extracted_entities": []entity.ExtractedEntity{
					{Kind: kleio.EntityKindFile, Value: "internal/auth/token.go", Confidence: 0.8},
				},
			},
		},
		{
			SourceID: "sig-2",
			Metadata: map[string]any{
				"extracted_entities": []entity.ExtractedEntity{
					{Kind: kleio.EntityKindFile, Value: "internal/auth/token.go", Confidence: 0.8},
				},
			},
		},
	}

	clusters, err := c.Correlate(context.Background(), signals)
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	assert.InDelta(t, 0.70, clusters[0].Confidence, 0.01)
}

func TestCorrelator_NilStore(t *testing.T) {
	c := New(nil)
	clusters, err := c.Correlate(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, clusters)
}
