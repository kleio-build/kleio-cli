// Package timewindow implements a kleio.Correlator that buckets RawSignals
// into clusters by their Timestamp. Signals whose timestamps fall within
// the same fixed window are grouped together; later passes can refine the
// grouping via ID or file-path overlap.
//
// Why a separate package vs. inlining in pipeline.Run?
//
//   - Each correlator is independently testable and swappable. The
//     pipeline never reaches inside; it consumes whatever Cluster slice
//     the Correlator returns.
//   - Time-window logic alone yields useful clusters even when no other
//     correlators are wired (the simplest pipeline configuration), so
//     this package is a self-contained MVP.
package timewindow

import (
	"context"
	"fmt"
	"sort"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

// Correlator groups RawSignals into clusters whose timestamps fall in
// the same Window-sized bucket. The anchor of each cluster is the
// signal with the highest "anchor weight" (plan umbrella > plan
// decision/checkpoint > anything else). When ties exist, the earliest
// timestamp wins so re-runs are deterministic.
type Correlator struct {
	// Window is the bucket size. Defaults to DefaultWindow (15 min).
	Window time.Duration

	// MinClusterSize gates which buckets are emitted as Clusters. A
	// bucket with fewer signals than MinClusterSize is dropped. A
	// value of 1 means "every signal is its own cluster" (rarely
	// useful); the default of 2 ensures some kind of relation exists.
	MinClusterSize int
}

// DefaultWindow is the time window used when Correlator.Window is zero.
// 15 minutes captures most "session" clusters (a developer working on
// one feature for a quarter hour) without merging across distinct work.
const DefaultWindow = 15 * time.Minute

func New(window time.Duration) *Correlator {
	return &Correlator{Window: window, MinClusterSize: 2}
}

func (c *Correlator) Name() string { return "time_window" }

// Correlate returns one Cluster per non-empty time bucket. Signals are
// bucketed by (RepoName, floor(Timestamp/Window)) so cross-repo signals
// at the same wall-clock time stay in distinct clusters; this prevents
// a transcript edited in repo A from polluting the cluster for repo B.
//
// When RepoName is empty (e.g. early plan ingester output that hasn't
// been attributed yet), all such signals share the "" repo bucket --
// downstream correlators will refine attribution via plan filename or
// file-path overlap.
func (c *Correlator) Correlate(ctx context.Context, signals []kleio.RawSignal) ([]kleio.Cluster, error) {
	window := c.Window
	if window <= 0 {
		window = DefaultWindow
	}
	minSize := c.MinClusterSize
	if minSize <= 0 {
		minSize = 2
	}

	type key struct {
		repo   string
		bucket int64
	}
	buckets := map[key][]kleio.RawSignal{}

	for _, s := range signals {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if s.Timestamp.IsZero() {
			continue
		}
		k := key{
			repo:   s.RepoName,
			bucket: s.Timestamp.UTC().UnixNano() / window.Nanoseconds(),
		}
		buckets[k] = append(buckets[k], s)
	}

	keys := make([]key, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].repo != keys[j].repo {
			return keys[i].repo < keys[j].repo
		}
		return keys[i].bucket < keys[j].bucket
	})

	var clusters []kleio.Cluster
	for _, k := range keys {
		members := buckets[k]
		if len(members) < minSize {
			continue
		}
		sort.SliceStable(members, func(i, j int) bool {
			return members[i].Timestamp.Before(members[j].Timestamp)
		})
		anchor := pickAnchor(members)
		links := make([]kleio.ClusterLink, 0, len(members))
		for _, m := range members {
			if signalKey(m) == anchor {
				continue
			}
			links = append(links, kleio.ClusterLink{
				From:       anchor,
				To:         signalKey(m),
				LinkType:   kleio.LinkTypeCorrelatedWith,
				Confidence: 0.4,
				Reason:     fmt.Sprintf("time_window=%s", window),
			})
		}
		clusters = append(clusters, kleio.Cluster{
			AnchorID:   anchor,
			AnchorType: anchorTypeFor(members),
			Members:    members,
			Links:      links,
			Confidence: 0.4,
			Provenance: []string{c.Name()},
		})
	}
	return clusters, nil
}

// pickAnchor selects the canonical anchor signal for a cluster. Plans
// (cursor_plan source) outrank everything; within plans, umbrellas
// outrank section bullets; otherwise the earliest signal wins for
// determinism.
func pickAnchor(members []kleio.RawSignal) string {
	best := -1
	bestScore := -1
	for i, m := range members {
		score := 0
		switch m.SourceType {
		case "cursor_plan":
			score += 100
		case kleio.SourceTypeLocalGit:
			score += 50
		case kleio.SourceTypeCursorTranscript:
			score += 10
		}
		if m.SourceOffset == "umbrella" {
			score += 50
		}
		if score > bestScore {
			bestScore = score
			best = i
		}
	}
	if best < 0 {
		best = 0
	}
	return signalKey(members[best])
}

// anchorTypeFor returns the anchor's source type ("cursor_plan",
// "local_git", "cursor_transcript", ...). Used by the synthesizer
// dispatch table downstream.
func anchorTypeFor(members []kleio.RawSignal) string {
	if len(members) == 0 {
		return ""
	}
	anchorID := pickAnchor(members)
	for _, m := range members {
		if signalKey(m) == anchorID {
			return m.SourceType
		}
	}
	return members[0].SourceType
}

// signalKey returns a stable identifier for a RawSignal: SourceID is
// already unique-per-source by ingester contract, so we use it
// verbatim. Tests assert this is non-empty for every emitted signal.
func signalKey(s kleio.RawSignal) string {
	if s.SourceID != "" {
		return s.SourceID
	}
	// Last-resort fallback so the pipeline doesn't blow up on a
	// malformed signal: synthesise an ad-hoc key from offset+content
	// hash. Should never happen in practice; tests assert this branch
	// is unreachable for properly-built ingesters.
	return s.SourceType + ":" + s.SourceOffset
}
