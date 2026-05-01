package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/ai"
)

// Report is the structured output produced by all three trigger commands
// (trace, explain, incident). Renderers vary section emphasis per command.
type Report struct {
	Anchor          string              `json:"anchor"`
	Command         string              `json:"command"`
	GeneratedAt     time.Time           `json:"generated_at"`
	Subject         string              `json:"subject"`
	Decisions       []ReportDecision    `json:"decisions,omitempty"`
	OpenThreads     []ReportThread      `json:"open_threads,omitempty"`
	ThreadGroups    []ReportThreadGroup `json:"thread_groups,omitempty"`
	CodeChanges     []ReportChange      `json:"code_changes,omitempty"`
	EvidenceQuality EvidenceQuality     `json:"evidence_quality"`
	NextSteps       []string            `json:"next_steps,omitempty"`
	RawTimeline     []TimelineEntry     `json:"raw_timeline,omitempty"`
	Enriched        bool                `json:"enriched"`
}

// ReportThreadGroup groups related work items under a plan cluster.
// When no plan data exists, all threads go into a single group with
// empty PlanName/ClusterID.
type ReportThreadGroup struct {
	PlanName  string         `json:"plan_name,omitempty"`
	ClusterID string         `json:"cluster_id,omitempty"`
	Threads   []ReportThread `json:"threads"`
}

type ReportDecision struct {
	Content   string `json:"content"`
	Rationale string `json:"rationale,omitempty"`
	At        string `json:"at"`
	Source    string `json:"source,omitempty"`
}

type ReportThread struct {
	Content     string `json:"content"`
	Occurrences int    `json:"occurrences"`
	FirstSeen   string `json:"first_seen"`
	LastSeen    string `json:"last_seen"`
	Deferred    bool   `json:"deferred,omitempty"`
	ClusterID   string `json:"cluster_id,omitempty"`
}

type ReportChange struct {
	SHA     string   `json:"sha"`
	Date    string   `json:"date"`
	Subject string   `json:"subject"`
	Files   []string `json:"files,omitempty"`
}

type EvidenceQuality struct {
	SourceCounts    map[string]int `json:"source_counts"`
	HistoryFidelity string         `json:"history_fidelity"`
	Notes           []string       `json:"notes,omitempty"`
}

// ReportOptions configures adaptive report building.
type ReportOptions struct {
	Stats *CorpusStats
}

// BuildReport constructs a Report from timeline entries using purely
// heuristic logic (no LLM). Call Report.Enrich afterwards if a provider
// is available.
func (e *Engine) BuildReport(ctx context.Context, anchor, command string, entries []TimelineEntry) Report {
	return e.BuildReportWithOptions(ctx, anchor, command, entries, ReportOptions{})
}

// BuildReportWithOptions constructs a Report with adaptive behaviour
// driven by corpus statistics.
func (e *Engine) BuildReportWithOptions(ctx context.Context, anchor, command string, entries []TimelineEntry, opts ReportOptions) Report {
	r := Report{
		Anchor:      anchor,
		Command:     command,
		GeneratedAt: time.Now().UTC(),
		RawTimeline: entries,
	}

	typeCounts := map[string]int{}
	dateActivity := map[string]int{}
	seenSHAs := map[string]bool{}
	threadHashes := map[string]int{}
	threadFirst := map[string]time.Time{}
	threadLast := map[string]time.Time{}
	threadContent := map[string]string{}
	threadCluster := map[string]string{}

	for _, entry := range entries {
		typeCounts[entry.Kind]++
		date := entry.Timestamp.Format("2006-01-02")
		dateActivity[date]++

		switch entry.Kind {
		case kleio.SignalTypeDecision:
			dec := ReportDecision{
				Content: entry.Summary,
				At:      entry.Timestamp.Format(time.RFC3339),
			}
			if entry.EventID != "" {
				ev, err := e.store.GetEvent(ctx, entry.EventID)
				if err == nil && ev.StructuredData != "" {
					var sd map[string]interface{}
					if json.Unmarshal([]byte(ev.StructuredData), &sd) == nil {
						if rat, ok := sd["rationale"].(string); ok {
							dec.Rationale = rat
						}
					}
					dec.Source = ev.SourceType
				}
			}
			r.Decisions = append(r.Decisions, dec)

		case kleio.SignalTypeWorkItem:
			h := contentHash(entry.Summary)
			threadHashes[h]++
			ts := entry.Timestamp
			if prev, ok := threadFirst[h]; !ok || ts.Before(prev) {
				threadFirst[h] = ts
			}
			if prev, ok := threadLast[h]; !ok || ts.After(prev) {
				threadLast[h] = ts
			}
			if existing, ok := threadContent[h]; !ok || len(entry.Summary) > len(existing) {
				threadContent[h] = entry.Summary
			}
			if entry.EventID != "" {
				if cid := extractClusterID(ctx, e.store, entry.EventID); cid != "" {
					threadCluster[h] = cid
				}
			}

		case kleio.SignalTypeGitCommit:
			if seenSHAs[entry.SHA] {
				continue
			}
			seenSHAs[entry.SHA] = true
			r.CodeChanges = append(r.CodeChanges, ReportChange{
				SHA:     entry.SHA,
				Date:    entry.Timestamp.Format("2006-01-02"),
				Subject: entry.Summary,
				Files:   entry.FilePaths,
			})
		}
	}

	for h, count := range threadHashes {
		lower := strings.ToLower(threadContent[h])
		deferred := strings.Contains(lower, "defer") ||
			strings.Contains(lower, "out of scope") ||
			strings.Contains(lower, "punt") ||
			strings.Contains(lower, "skip") ||
			strings.Contains(lower, "backlog")
		r.OpenThreads = append(r.OpenThreads, ReportThread{
			Content:     threadContent[h],
			Occurrences: count,
			FirstSeen:   threadFirst[h].Format(time.RFC3339),
			LastSeen:    threadLast[h].Format(time.RFC3339),
			Deferred:    deferred,
			ClusterID:   threadCluster[h],
		})
	}
	sort.Slice(r.OpenThreads, func(i, j int) bool {
		return r.OpenThreads[i].Occurrences > r.OpenThreads[j].Occurrences
	})

	isSignalRich := false
	if opts.Stats != nil {
		isSignalRich = opts.Stats.IsSignalRich
	}
	r.ThreadGroups = groupThreadsByCluster(r.OpenThreads, isSignalRich)

	r.Subject = buildSubject(anchor, entries, typeCounts, dateActivity)
	r.EvidenceQuality = buildEvidenceQuality(ctx, e.store, entries, r.OpenThreads)
	r.NextSteps = buildNextSteps(anchor, r.CodeChanges)

	return r
}

func extractClusterID(ctx context.Context, store kleio.Store, eventID string) string {
	ev, err := store.GetEvent(ctx, eventID)
	if err != nil || ev.StructuredData == "" {
		return ""
	}
	var sd map[string]interface{}
	if json.Unmarshal([]byte(ev.StructuredData), &sd) != nil {
		return ""
	}
	if cid, ok := sd[kleio.StructuredKeyClusterAnchorID].(string); ok {
		return cid
	}
	return ""
}

func groupThreadsByCluster(threads []ReportThread, signalRich bool) []ReportThreadGroup {
	if !signalRich {
		return []ReportThreadGroup{{Threads: threads}}
	}

	grouped := map[string][]ReportThread{}
	var order []string
	for _, t := range threads {
		key := t.ClusterID
		if key == "" {
			key = "__orphan__"
		}
		if _, exists := grouped[key]; !exists {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], t)
	}

	hasMultipleGroups := false
	for _, key := range order {
		if key != "__orphan__" {
			hasMultipleGroups = true
			break
		}
	}
	if !hasMultipleGroups {
		return []ReportThreadGroup{{Threads: threads}}
	}

	var groups []ReportThreadGroup
	for _, key := range order {
		g := ReportThreadGroup{
			Threads: grouped[key],
		}
		if key != "__orphan__" {
			g.ClusterID = key
			g.PlanName = "Plan: " + key
		} else {
			g.PlanName = "Other signals"
		}
		groups = append(groups, g)
	}
	return groups
}

// Enrich uses an LLM provider to improve the heuristic report.
func (r *Report) Enrich(ctx context.Context, p ai.Provider) error {
	if !p.Available() {
		return nil
	}

	if subj, err := enrichSubject(ctx, p, r); err == nil && subj != "" {
		r.Subject = subj
	}

	if len(r.OpenThreads) > 1 {
		enrichDedupThreads(ctx, p, r)
	}

	if steps, err := enrichNextSteps(ctx, p, r); err == nil && len(steps) > 0 {
		r.NextSteps = steps
	}

	r.Enriched = true
	return nil
}

func enrichSubject(ctx context.Context, p ai.Provider, r *Report) (string, error) {
	prompt := fmt.Sprintf(
		"Summarize in 2-3 sentences for an engineering audience. "+
			"Context: %d decisions, %d open threads, %d code changes.\n"+
			"Current summary: %s",
		len(r.Decisions), len(r.OpenThreads), len(r.CodeChanges), r.Subject)
	return p.Complete(ctx, prompt)
}

func enrichDedupThreads(ctx context.Context, p ai.Provider, r *Report) {
	type vecThread struct {
		idx int
		vec []float64
	}
	var vecs []vecThread
	for i, t := range r.OpenThreads {
		v, err := p.Embed(ctx, t.Content)
		if err != nil || len(v) == 0 {
			continue
		}
		vecs = append(vecs, vecThread{idx: i, vec: v})
	}
	if len(vecs) < 2 {
		return
	}

	merged := map[int]bool{}
	for i := 0; i < len(vecs); i++ {
		if merged[vecs[i].idx] {
			continue
		}
		for j := i + 1; j < len(vecs); j++ {
			if merged[vecs[j].idx] {
				continue
			}
			if cosineSim(vecs[i].vec, vecs[j].vec) >= 0.85 {
				src := &r.OpenThreads[vecs[i].idx]
				dup := r.OpenThreads[vecs[j].idx]
				src.Occurrences += dup.Occurrences
				if dup.FirstSeen < src.FirstSeen {
					src.FirstSeen = dup.FirstSeen
				}
				if dup.LastSeen > src.LastSeen {
					src.LastSeen = dup.LastSeen
				}
				if len(dup.Content) > len(src.Content) {
					src.Content = dup.Content
				}
				src.Deferred = src.Deferred || dup.Deferred
				merged[vecs[j].idx] = true
			}
		}
	}

	if len(merged) > 0 {
		var kept []ReportThread
		for i, t := range r.OpenThreads {
			if !merged[i] {
				kept = append(kept, t)
			}
		}
		r.OpenThreads = kept
	}
}

func enrichNextSteps(ctx context.Context, p ai.Provider, r *Report) ([]string, error) {
	var sb strings.Builder
	sb.WriteString("Given this engineering context, suggest 2-3 follow-up kleio CLI commands:\n")
	sb.WriteString("Subject: " + r.Subject + "\n")
	if len(r.Decisions) > 0 {
		sb.WriteString("Decisions: ")
		for _, d := range r.Decisions {
			sb.WriteString(d.Content + "; ")
		}
		sb.WriteString("\n")
	}
	if len(r.OpenThreads) > 0 {
		sb.WriteString("Open threads: ")
		for _, t := range r.OpenThreads {
			sb.WriteString(t.Content + "; ")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Available commands: kleio trace <topic>, kleio explain <from> <to>, kleio incident <signal>, kleio backlog list --search <term>\n")
	sb.WriteString("Reply with ONLY the commands, one per line, no explanation.")

	result, err := p.Complete(ctx, sb.String())
	if err != nil || result == "" {
		return nil, err
	}

	var steps []string
	for _, line := range strings.Split(result, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "kleio ") {
			steps = append(steps, line)
		}
	}
	if len(steps) == 0 {
		return nil, nil
	}
	return steps, nil
}

func buildSubject(anchor string, entries []TimelineEntry, typeCounts map[string]int, dateActivity map[string]int) string {
	total := len(entries)
	if total == 0 {
		return fmt.Sprintf("No signals found for %q.", anchor)
	}

	busiestDate := ""
	busiestCount := 0
	for d, c := range dateActivity {
		if c > busiestCount {
			busiestDate = d
			busiestCount = c
		}
	}

	var parts []string
	if n := typeCounts[kleio.SignalTypeDecision]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d decision(s)", n))
	}
	if n := typeCounts[kleio.SignalTypeWorkItem]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d work item(s)", n))
	}
	if n := typeCounts[kleio.SignalTypeGitCommit]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d commit(s)", n))
	}
	if n := typeCounts[kleio.SignalTypeCheckpoint]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d checkpoint(s)", n))
	}

	subj := fmt.Sprintf("%d signals tied to %q", total, anchor)
	if len(parts) > 0 {
		subj += ": " + strings.Join(parts, ", ")
	}
	if busiestCount > 1 && busiestDate != "" {
		subj += fmt.Sprintf(". Most activity (%d/%d) on %s", busiestCount, total, busiestDate)
	}
	return subj + "."
}

func buildEvidenceQuality(ctx context.Context, store kleio.Store, entries []TimelineEntry, threads []ReportThread) EvidenceQuality {
	eq := EvidenceQuality{
		SourceCounts:    map[string]int{},
		HistoryFidelity: "high",
	}

	for _, entry := range entries {
		if entry.EventID != "" {
			ev, err := store.GetEvent(ctx, entry.EventID)
			if err == nil {
				eq.SourceCounts[ev.SourceType]++
			}
		} else if entry.SHA != "" {
			eq.SourceCounts["local_git"]++
		}
	}

	dupCount := 0
	for _, t := range threads {
		if t.Occurrences > 1 {
			dupCount++
		}
	}
	if dupCount > 0 {
		eq.Notes = append(eq.Notes,
			fmt.Sprintf("%d work item(s) appear duplicated across re-imported transcripts.", dupCount))
	}

	return eq
}

func buildNextSteps(anchor string, changes []ReportChange) []string {
	var steps []string
	if len(changes) > 0 {
		oldest := changes[len(changes)-1].SHA
		if len(oldest) > 7 {
			oldest = oldest[:7]
		}
		steps = append(steps, fmt.Sprintf("kleio explain %s HEAD", oldest))
	}
	steps = append(steps, fmt.Sprintf("kleio backlog list --search %q", anchor))
	return steps
}

func contentHash(s string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(s))))
	return hex.EncodeToString(h[:8])
}

func cosineSim(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
