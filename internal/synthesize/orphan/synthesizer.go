// Package orphan implements a kleio.Synthesizer for clusters that
// have NO plan anchor. The plan-anchored path (PlanClusterSynthesizer)
// is the high-trust route; this package is the conservative fallback
// that promotes only the "explicit and unambiguous" subset of
// transcript and git signals.
//
// Promote rules (anything not on this list is dropped):
//
//   - kleio_capture / kleio_decide / kleio_checkpoint MCP tool calls
//     (transcript SourceOffset starts with "toolcall:")
//   - explicit user retro-asks (transcript SourceOffset starts with
//     "retro_ask:") -- the user told us to track them
//   - git commits whose message starts with a conventional-commit type
//     (feat:, fix:, perf:, refactor:, docs:, test:, build:, chore:)
//
// Everything else is intentionally rejected. The whole point of this
// package is "if it isn't a plan or an explicit signal, we don't have
// enough confidence to call it a real capture."
package orphan

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

type Synthesizer struct{}

func New() *Synthesizer { return &Synthesizer{} }

func (*Synthesizer) Name() string { return "orphan" }

// conventionalCommitRe matches the prefix of a Conventional Commit.
// Optional scope and bang are accepted; case-insensitive.
var conventionalCommitRe = regexp.MustCompile(`^(?i)(feat|fix|perf|refactor|docs|test|build|chore|ci|style)(\([^)]+\))?!?:\s+`)

// Synthesize returns Events for orphan clusters. Returns no Events
// for plan-anchored clusters (those go through PlanClusterSynthesizer).
func (s *Synthesizer) Synthesize(ctx context.Context, cluster kleio.Cluster) ([]kleio.Event, error) {
	if cluster.AnchorType == "cursor_plan" {
		return nil, nil
	}

	seen := map[string]bool{}
	var events []kleio.Event
	for _, m := range cluster.Members {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		ev, ok := promote(m, cluster)
		if !ok {
			continue
		}
		dedupKey := ev.SignalType + "|" + ev.Content
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true
		events = append(events, ev)
	}
	return events, nil
}

// promote inspects one cluster member and either returns the Event it
// should become (ok=true) or rejects it (ok=false).
func promote(m kleio.RawSignal, cluster kleio.Cluster) (kleio.Event, bool) {
	switch m.SourceType {
	case kleio.SourceTypeCursorTranscript:
		return promoteTranscript(m, cluster)
	case kleio.SourceTypeLocalGit:
		return promoteCommit(m, cluster)
	}
	return kleio.Event{}, false
}

func promoteTranscript(m kleio.RawSignal, cluster kleio.Cluster) (kleio.Event, bool) {
	switch {
	case strings.HasPrefix(m.SourceOffset, "toolcall:"):
		return buildEvent(m, cluster, m.Kind, "agent", map[string]any{
			"explicit": true,
			"channel":  "kleio_mcp",
		}), true
	case strings.HasPrefix(m.SourceOffset, "retro_ask:"):
		return buildEvent(m, cluster, kleio.SignalTypeWorkItem, "user", map[string]any{
			"channel": "user_retro_ask",
		}), true
	}
	return kleio.Event{}, false
}

func promoteCommit(m kleio.RawSignal, cluster kleio.Cluster) (kleio.Event, bool) {
	if !conventionalCommitRe.MatchString(m.Content) {
		return kleio.Event{}, false
	}
	branch, _ := m.Metadata["branch"].(string)
	ev := buildEvent(m, cluster, kleio.SignalTypeGitCommit, "user", map[string]any{
		"channel": "git_commit",
		"branch":  branch,
	})
	ev.BranchName = branch
	return ev, true
}

func buildEvent(m kleio.RawSignal, cluster kleio.Cluster, signalType, authorType string, extraSD map[string]any) kleio.Event {
	created := m.Timestamp
	if created.IsZero() {
		created = time.Now().UTC()
	}
	sd := map[string]any{
		kleio.StructuredKeyClusterAnchorID: cluster.AnchorID,
		kleio.StructuredKeyParentSignalID:  cluster.AnchorID,
		kleio.StructuredKeyProvenance:      "orphan",
		"source_offset":                    m.SourceOffset,
	}
	for k, v := range extraSD {
		sd[k] = v
	}
	if rat, ok := m.Metadata["rationale"].(string); ok && rat != "" {
		sd["rationale"] = rat
	}
	sdBytes, _ := json.Marshal(sd)

	return kleio.Event{
		ID:             fmt.Sprintf("orphan:%s", m.SourceID),
		SignalType:     signalType,
		Content:        truncate(m.Content, 1500),
		SourceType:     m.SourceType,
		CreatedAt:      created.UTC().Format(time.RFC3339),
		RepoName:       m.RepoName,
		StructuredData: string(sdBytes),
		AuthorType:     authorType,
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
