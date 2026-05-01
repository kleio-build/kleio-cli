package plan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const wellFormedPlan = `---
name: Test Plan
overview: Demo plan with full structure for golden tests.
todos:
  - id: t1
    content: First todo
    status: pending
  - id: t2
    content: Second todo
    status: completed
isProject: false
---

# Body

## Section A

Some prose here.

**Decision:** Adopt approach X
rationale: Stateless wins.

### Decision: Use library Y

## Out of scope

- Defer caching to a follow-up
- Skip rate limiting

## Future enhancements

- Slack ingester (next quarter)

## Risks

- Latency under load
- Auth token refresh edge case

## Open Questions

- How do we handle multi-tenant?

## Implementation

This shouldn't be parsed as anything special.
`

func TestParseFile_Roundtrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.plan.md")
	require.NoError(t, os.WriteFile(path, []byte(wellFormedPlan), 0o644))

	pp, err := ParseFile(path)
	require.NoError(t, err)

	assert.Equal(t, "Test Plan", pp.Frontmatter.Name)
	assert.Len(t, pp.Frontmatter.Todos, 2)
	assert.Equal(t, "t1", pp.Frontmatter.Todos[0].ID)
	assert.Equal(t, "First todo", pp.Frontmatter.Todos[0].Content)
	assert.Equal(t, "pending", pp.Frontmatter.Todos[0].Status)
	assert.Contains(t, pp.Body, "## Section A")
	assert.Greater(t, pp.BodyOffset, 1)
}

func TestSignalsFromPlan_EmitsAllPasses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.plan.md")
	require.NoError(t, os.WriteFile(path, []byte(wellFormedPlan), 0o644))

	pp, err := ParseFile(path)
	require.NoError(t, err)

	signals := SignalsFromPlan(pp, "test-repo")

	counts := bucketBySourceOffsetPrefix(signals)
	assert.Equal(t, 2, counts["todo:"], "two todos in frontmatter")
	assert.GreaterOrEqual(t, counts["decision:"], 1, "at least one decision block")
	assert.Equal(t, 3, counts["deferred:"], "two out-of-scope + one future bullet")
	assert.GreaterOrEqual(t, counts["risk:"], 2, "two risks + one open question")
	assert.Equal(t, 1, counts["umbrella:"], "exactly one umbrella signal")

	for _, s := range signals {
		assert.Equal(t, "cursor_plan", s.SourceType, "every signal sourced from plan")
		assert.Equal(t, "test-repo", s.RepoName, "RepoName propagated")
		assert.NotEmpty(t, s.SourceID, "every signal has a stable SourceID")
	}
}

func TestSignalsFromPlan_DeferredFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.plan.md")
	require.NoError(t, os.WriteFile(path, []byte(wellFormedPlan), 0o644))
	pp, _ := ParseFile(path)
	signals := SignalsFromPlan(pp, "")

	for _, s := range signals {
		if !strings.HasPrefix(s.SourceOffset, "deferred:") {
			continue
		}
		assert.True(t, s.Metadata["deferred"].(bool), "deferred metadata flag must be true")
	}
}

func TestSignalsFromPlan_UmbrellaIsAnchor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.plan.md")
	require.NoError(t, os.WriteFile(path, []byte(wellFormedPlan), 0o644))
	pp, _ := ParseFile(path)
	signals := SignalsFromPlan(pp, "")

	var umbrellas int
	for _, s := range signals {
		if s.SourceOffset == "umbrella" {
			umbrellas++
			assert.True(t, s.Metadata["is_anchor"].(bool))
			assert.Contains(t, s.Content, "Test Plan")
			assert.Equal(t, kleio.SignalTypeCheckpoint, s.Kind)
		}
	}
	assert.Equal(t, 1, umbrellas)
}

func TestSignalsFromPlan_HandlesMissingFrontmatter(t *testing.T) {
	noFrontmatter := `# A plan with no frontmatter

## Out of scope
- Just one bullet
`
	dir := t.TempDir()
	path := filepath.Join(dir, "noyaml.plan.md")
	require.NoError(t, os.WriteFile(path, []byte(noFrontmatter), 0o644))
	pp, err := ParseFile(path)
	require.NoError(t, err)

	signals := SignalsFromPlan(pp, "")
	counts := bucketBySourceOffsetPrefix(signals)
	assert.Equal(t, 0, counts["todo:"], "no todos when frontmatter missing")
	assert.Equal(t, 1, counts["deferred:"], "still parses body bullets")
	assert.Equal(t, 1, counts["umbrella:"], "umbrella always emitted, falls back to filename")
}

func TestSignalsFromPlan_HandlesMalformedYAML(t *testing.T) {
	bad := `---
name: Bad
todos:
  - this isn't valid yaml structure
    should: probably: still: parse: somehow
---

## Out of scope
- One thing
`
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.plan.md")
	require.NoError(t, os.WriteFile(path, []byte(bad), 0o644))
	pp, err := ParseFile(path)
	require.NoError(t, err)

	signals := SignalsFromPlan(pp, "")
	counts := bucketBySourceOffsetPrefix(signals)
	assert.Equal(t, 1, counts["umbrella:"], "still emits umbrella despite bad YAML")
	assert.Equal(t, 1, counts["deferred:"], "body bullets still parsed")
}

func TestSignalsFromPlan_DecisionWithRationale(t *testing.T) {
	plan := `---
name: D
---

**Decision:** Pick A
rationale: It's faster.

**Decision:** Pick B
`
	dir := t.TempDir()
	path := filepath.Join(dir, "d.plan.md")
	require.NoError(t, os.WriteFile(path, []byte(plan), 0o644))
	pp, _ := ParseFile(path)
	signals := SignalsFromPlan(pp, "")

	var decisions []kleio.RawSignal
	for _, s := range signals {
		if strings.HasPrefix(s.SourceOffset, "decision:") {
			decisions = append(decisions, s)
		}
	}
	require.Len(t, decisions, 2)
	assert.Equal(t, "Pick A", decisions[0].Content)
	assert.Equal(t, "It's faster.", decisions[0].Metadata["rationale"])
	assert.Equal(t, "Pick B", decisions[1].Content)
	_, hasRat := decisions[1].Metadata["rationale"]
	assert.False(t, hasRat, "no rationale extracted when none present")
}

func TestIngester_RoundtripDeterministic(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, ".cursor", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "a.plan.md"), []byte(wellFormedPlan), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "b.plan.md"), []byte(wellFormedPlan), 0o644))

	ing := New(dir)
	ctx := context.Background()
	first, err := ing.Ingest(ctx, kleio.IngestScope{RepoName: "demo"})
	require.NoError(t, err)
	second, err := ing.Ingest(ctx, kleio.IngestScope{RepoName: "demo"})
	require.NoError(t, err)
	assert.Equal(t, len(first), len(second))
	for i := range first {
		assert.Equal(t, first[i].SourceID, second[i].SourceID, "deterministic ordering across runs")
	}
}

func TestSignalsFromPlan_EmptyBodyOnlyEmitsUmbrella(t *testing.T) {
	emptyBody := `---
name: Empty
---
`
	dir := t.TempDir()
	path := filepath.Join(dir, "e.plan.md")
	require.NoError(t, os.WriteFile(path, []byte(emptyBody), 0o644))
	pp, _ := ParseFile(path)
	signals := SignalsFromPlan(pp, "")
	counts := bucketBySourceOffsetPrefix(signals)
	assert.Equal(t, 0, counts["todo:"])
	assert.Equal(t, 0, counts["decision:"])
	assert.Equal(t, 0, counts["deferred:"])
	assert.Equal(t, 0, counts["risk:"])
	assert.Equal(t, 1, counts["umbrella:"], "umbrella always emitted")
}

func TestSignalsFromPlan_FrontmatterTodosOnly(t *testing.T) {
	plan := `---
name: Todos Only
todos:
  - id: a
    content: First
    status: pending
  - id: b
    content: Second
    status: in_progress
  - id: c
    content: Third
    status: completed
---

# Just a title.
`
	dir := t.TempDir()
	path := filepath.Join(dir, "t.plan.md")
	require.NoError(t, os.WriteFile(path, []byte(plan), 0o644))
	pp, _ := ParseFile(path)
	signals := SignalsFromPlan(pp, "")
	counts := bucketBySourceOffsetPrefix(signals)
	assert.Equal(t, 3, counts["todo:"])
	for _, s := range signals {
		if !strings.HasPrefix(s.SourceOffset, "todo:") {
			continue
		}
		status, ok := s.Metadata["status"].(string)
		if !ok {
			t.Fatalf("todo missing status metadata: %#v", s)
		}
		assert.Contains(t, []string{"pending", "in_progress", "completed"}, status)
	}
}

func TestSignalsFromPlan_DeterministicAcrossInvocations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "det.plan.md")
	require.NoError(t, os.WriteFile(path, []byte(wellFormedPlan), 0o644))
	pp, _ := ParseFile(path)

	a := SignalsFromPlan(pp, "demo")
	b := SignalsFromPlan(pp, "demo")
	require.Equal(t, len(a), len(b))
	for i := range a {
		assert.Equal(t, a[i].SourceID, b[i].SourceID)
		assert.Equal(t, a[i].Content, b[i].Content)
		assert.Equal(t, a[i].SourceOffset, b[i].SourceOffset)
	}
}

func TestSignalsFromPlan_LongContentPreserved(t *testing.T) {
	// Plans are author intent: long deferred items must round-trip
	// without truncation so the synthesizer / report layer have the
	// full text to summarize. (Renderers are responsible for visual
	// wrapping, not the ingester.)
	long := strings.Repeat("a really very long line of plan content. ", 20)
	plan := "---\nname: Long\n---\n## Out of scope\n- " + long + "\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "long.plan.md")
	require.NoError(t, os.WriteFile(path, []byte(plan), 0o644))
	pp, _ := ParseFile(path)
	signals := SignalsFromPlan(pp, "")
	var foundDeferred bool
	for _, s := range signals {
		if !strings.HasPrefix(s.SourceOffset, "deferred:") {
			continue
		}
		foundDeferred = true
		assert.Contains(t, s.Content, "a really very long line", "long deferred content preserved verbatim")
	}
	assert.True(t, foundDeferred)
}

func TestDiscoverPlansDir_WalksUp(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".cursor", "plans"), 0o755))
	deep := filepath.Join(tmp, "a", "b", "c")
	require.NoError(t, os.MkdirAll(deep, 0o755))

	got := DiscoverPlansDir(deep)
	assert.Equal(t, filepath.Join(tmp, ".cursor", "plans"), got)
}

func bucketBySourceOffsetPrefix(signals []kleio.RawSignal) map[string]int {
	out := map[string]int{}
	for _, s := range signals {
		switch {
		case strings.HasPrefix(s.SourceOffset, "todo:"):
			out["todo:"]++
		case strings.HasPrefix(s.SourceOffset, "decision:"):
			out["decision:"]++
		case strings.HasPrefix(s.SourceOffset, "deferred:"):
			out["deferred:"]++
		case strings.HasPrefix(s.SourceOffset, "risk:"):
			out["risk:"]++
		case s.SourceOffset == "umbrella":
			out["umbrella:"]++
		}
	}
	return out
}
