// Package llm implements an OPTIONAL kleio.Synthesizer that runs a
// final pass over each cluster, asking the LLM to:
//
//  1. Choose the best one-line subject for the cluster
//  2. Write a 1-2 sentence narrative summary
//  3. Flag low-confidence signals it would drop
//
// The output is attached as StructuredData on a single "summary"
// Event per cluster. The original Events from PlanClusterSynthesizer
// or OrphanSynthesizer are NOT modified -- this stage is purely
// additive. When ai.AutoDetect returns no provider, this synthesizer
// is a no-op.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kleio-build/kleio-cli/internal/ai"
	kleio "github.com/kleio-build/kleio-core"
)

type Synthesizer struct {
	Provider ai.Provider

	// ConfidenceThreshold gates which clusters get LLM synthesis.
	// Clusters at or above this confidence are considered already
	// well-understood by deterministic synthesizers and are skipped.
	// Default 0.7 per the plan specification.
	ConfidenceThreshold float64

	// MaxClusterMembers caps the number of cluster members included in
	// the prompt to keep token usage manageable. Default 30.
	MaxClusterMembers int

	// MaxContentChars caps each member's quoted content. Default 200.
	MaxContentChars int
}

func New(provider ai.Provider) *Synthesizer {
	return &Synthesizer{
		Provider:            provider,
		ConfidenceThreshold: 0.7,
		MaxClusterMembers:   30,
		MaxContentChars:     200,
	}
}

func (*Synthesizer) Name() string { return "llm" }

func (s *Synthesizer) Available() bool {
	return s.Provider != nil && s.Provider.Available()
}

// Synthesize returns at most one Event per cluster: a "summary"
// capture with the LLM-distilled subject and narrative attached as
// StructuredData. Returns nil when LLM is unavailable.
func (s *Synthesizer) Synthesize(ctx context.Context, cluster kleio.Cluster) ([]kleio.Event, error) {
	if !s.Available() {
		return nil, nil
	}
	if len(cluster.Members) == 0 {
		return nil, nil
	}
	if cluster.Confidence >= s.ConfidenceThreshold {
		return nil, nil
	}
	prompt := buildPrompt(cluster, s.MaxClusterMembers, s.MaxContentChars)
	if prompt == "" {
		return nil, nil
	}
	resp, err := s.Provider.Complete(ctx, prompt)
	if err != nil {
		return nil, nil
	}
	subject, narrative := parseLLMResponse(resp)
	if subject == "" && narrative == "" {
		return nil, nil
	}

	created := time.Now().UTC()
	for _, m := range cluster.Members {
		if !m.Timestamp.IsZero() {
			created = m.Timestamp.UTC()
			break
		}
	}

	sd, _ := json.Marshal(map[string]any{
		kleio.StructuredKeyClusterAnchorID: cluster.AnchorID,
		kleio.StructuredKeyParentSignalID:  cluster.AnchorID,
		kleio.StructuredKeyProvenance:      "llm",
		"subject":                          subject,
		"narrative":                        narrative,
		"members_count":                    len(cluster.Members),
	})

	repo := ""
	for _, m := range cluster.Members {
		if m.RepoName != "" {
			repo = m.RepoName
			break
		}
	}

	return []kleio.Event{{
		ID:             fmt.Sprintf("llm:%s", cluster.AnchorID),
		SignalType:     kleio.SignalTypeCheckpoint,
		Content:        chooseContent(subject, narrative),
		SourceType:     "llm_summary",
		CreatedAt:      created.Format(time.RFC3339),
		RepoName:       repo,
		StructuredData: string(sd),
		AuthorType:     "agent",
		AuthorLabel:    "llm",
	}}, nil
}

// buildPrompt assembles a deterministic prompt that asks the model to
// distill the cluster into a one-line subject and a short narrative.
// We use a simple two-line response format ("SUBJECT: ...\nNARRATIVE:
// ...") instead of JSON because small local models often produce
// invalid JSON.
func buildPrompt(cluster kleio.Cluster, maxMembers, maxContentChars int) string {
	if len(cluster.Members) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("You are a software engineering analyst. Below is a cluster of related signals from a developer's session (plans, transcripts, commits). Your job is to summarize what the developer was doing in this cluster.\n\n")
	b.WriteString("Output EXACTLY two lines and nothing else:\n")
	b.WriteString("SUBJECT: <one-line title (5-12 words)>\n")
	b.WriteString("NARRATIVE: <1-2 sentence narrative summary>\n\n")
	b.WriteString("Cluster anchor type: ")
	b.WriteString(cluster.AnchorType)
	b.WriteString("\nMembers:\n")

	limit := maxMembers
	if limit > len(cluster.Members) {
		limit = len(cluster.Members)
	}
	for i := 0; i < limit; i++ {
		m := cluster.Members[i]
		content := strings.ReplaceAll(m.Content, "\n", " ")
		if len(content) > maxContentChars {
			content = content[:maxContentChars] + "..."
		}
		b.WriteString(fmt.Sprintf("- [%s/%s] %s\n", m.SourceType, m.SourceOffset, content))
	}
	if len(cluster.Members) > limit {
		b.WriteString(fmt.Sprintf("- (...%d more members truncated)\n", len(cluster.Members)-limit))
	}
	return b.String()
}

func parseLLMResponse(resp string) (subject, narrative string) {
	for _, line := range strings.Split(resp, "\n") {
		trimmed := strings.TrimSpace(line)
		if subj, ok := strings.CutPrefix(trimmed, "SUBJECT:"); ok {
			subject = strings.TrimSpace(subj)
			continue
		}
		if narr, ok := strings.CutPrefix(trimmed, "NARRATIVE:"); ok {
			narrative = strings.TrimSpace(narr)
			continue
		}
	}
	return subject, narrative
}

func chooseContent(subject, narrative string) string {
	if subject != "" && narrative != "" {
		return subject + " -- " + narrative
	}
	if subject != "" {
		return subject
	}
	return narrative
}
