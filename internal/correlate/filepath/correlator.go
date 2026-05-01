// Package filepath implements a kleio.Correlator that groups signals
// touching overlapping file paths.
//
// Heuristic: two signals share a "file" relation when:
//   - both touch the same exact path (highest confidence)
//   - both touch sibling files in the same directory under a depth
//     budget (lower confidence; gated by SharedDirThreshold)
//
// Plans typically don't carry file paths in their metadata, so this
// correlator mostly matches transcript edits ↔ git commits. Plan→file
// matches happen only when the plan body references a path verbatim
// (handled by extracting `path/to/file` patterns from Content).
package filepath

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	kleio "github.com/kleio-build/kleio-core"
)

// Correlator implements kleio.Correlator with two passes:
//  1. exact-path overlap (LinkType = touches, confidence = 0.75)
//  2. shared-directory overlap with depth >= SharedDirThreshold
//     (LinkType = touches, confidence = 0.45)
type Correlator struct {
	// SharedDirThreshold is the minimum number of common path segments
	// that two signals must share before a "shared directory" link is
	// emitted. Default 3 keeps "internal/ingest/plan/foo.go" and
	// "internal/ingest/plan/bar.go" together while excluding overly
	// generic matches like "internal/X" vs "internal/Y".
	SharedDirThreshold int

	// MinClusterSize gates emission. Default 2 (a file referenced by
	// only one signal yields no cluster).
	MinClusterSize int
}

func New() *Correlator {
	return &Correlator{SharedDirThreshold: 3, MinClusterSize: 2}
}

func (c *Correlator) Name() string { return "file_path" }

// Path-extract regex: matches things that look like file paths in
// markdown/prose (e.g. `internal/foo/bar.go`, `app/components/X.tsx`).
// Conservative: requires at least one '/' and a known-ish extension.
var inlinePathRe = regexp.MustCompile(`(?:\.[\\/]|[\w\-]+[\\/])(?:[\w\-]+[\\/])*[\w\-]+\.(go|ts|tsx|js|jsx|py|rs|rb|md|sql|sh|yaml|yml|json|toml|html|css)\b`)

// Correlate scans signals once, building file -> []signal indexes for
// both exact-path and directory-prefix matches. It then emits clusters
// using the higher-confidence relation when both apply.
func (c *Correlator) Correlate(ctx context.Context, signals []kleio.RawSignal) ([]kleio.Cluster, error) {
	minSize := c.MinClusterSize
	if minSize <= 0 {
		minSize = 2
	}
	depth := c.SharedDirThreshold
	if depth <= 0 {
		depth = 3
	}

	type sigEntry struct {
		key   string
		paths []string
	}
	entries := make([]sigEntry, 0, len(signals))
	signalsByKey := map[string]kleio.RawSignal{}
	exact := map[string]map[string]bool{}
	dirIndex := map[string]map[string]bool{}

	for _, s := range signals {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		paths := pathsFromSignal(s)
		if len(paths) == 0 {
			continue
		}
		key := signalKey(s)
		signalsByKey[key] = s
		entries = append(entries, sigEntry{key: key, paths: paths})

		seenDirs := map[string]bool{}
		for _, p := range paths {
			normalized := normalizePath(p)
			if normalized == "" {
				continue
			}
			if exact[normalized] == nil {
				exact[normalized] = map[string]bool{}
			}
			exact[normalized][key] = true

			d := dirOf(normalized, depth)
			if d != "" && !seenDirs[d] {
				seenDirs[d] = true
				if dirIndex[d] == nil {
					dirIndex[d] = map[string]bool{}
				}
				dirIndex[d][key] = true
			}
		}
	}

	used := map[string]bool{}
	var clusters []kleio.Cluster

	emit := func(name string, members map[string]bool, conf float64, reason string) {
		if len(members) < minSize {
			return
		}
		signature := clusterSignature(members)
		if used[signature] {
			return
		}
		used[signature] = true

		memberSlice := make([]kleio.RawSignal, 0, len(members))
		for k := range members {
			memberSlice = append(memberSlice, signalsByKey[k])
		}
		sort.Slice(memberSlice, func(i, j int) bool {
			return memberSlice[i].Timestamp.Before(memberSlice[j].Timestamp)
		})
		anchor := pickAnchor(memberSlice)
		links := make([]kleio.ClusterLink, 0, len(memberSlice)-1)
		for _, m := range memberSlice {
			if signalKey(m) == anchor {
				continue
			}
			links = append(links, kleio.ClusterLink{
				From:       anchor,
				To:         signalKey(m),
				LinkType:   kleio.LinkTypeTouches,
				Confidence: conf,
				Reason:     reason,
			})
		}
		clusters = append(clusters, kleio.Cluster{
			AnchorID:   anchor,
			AnchorType: anchorType(memberSlice, anchor),
			Members:    memberSlice,
			Links:      links,
			Confidence: conf,
			Provenance: []string{name},
		})
	}

	exactKeys := make([]string, 0, len(exact))
	for k := range exact {
		exactKeys = append(exactKeys, k)
	}
	sort.Strings(exactKeys)
	for _, k := range exactKeys {
		emit("file_path:exact="+k, exact[k], 0.75, fmt.Sprintf("exact_path=%s", k))
	}

	dirKeys := make([]string, 0, len(dirIndex))
	for k := range dirIndex {
		dirKeys = append(dirKeys, k)
	}
	sort.Strings(dirKeys)
	for _, k := range dirKeys {
		emit("file_path:dir="+k, dirIndex[k], 0.45, fmt.Sprintf("shared_dir=%s", k))
	}

	return clusters, nil
}

// pathsFromSignal extracts every plausible file path associated with a
// signal: the explicit Metadata["files"] list (git commits), explicit
// Metadata["file_path"] string (transcripts), and inline path-shaped
// substrings of Content (plans + transcripts).
func pathsFromSignal(s kleio.RawSignal) []string {
	var out []string
	if files, ok := s.Metadata["files"].([]string); ok {
		out = append(out, files...)
	}
	if files, ok := s.Metadata["files"].([]any); ok {
		for _, f := range files {
			if str, ok := f.(string); ok {
				out = append(out, str)
			}
		}
	}
	if fp, ok := s.Metadata["file_path"].(string); ok && fp != "" {
		out = append(out, fp)
	}
	if s.Content != "" {
		out = append(out, inlinePathRe.FindAllString(s.Content, -1)...)
	}
	return out
}

// normalizePath standardises separators to '/' and strips leading './'.
// File paths in different sources may differ stylistically; we want
// internal/foo/bar.go to match in every case.
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	return p
}

// dirOf returns the directory of p truncated to at most `depth`
// segments, or "" when p doesn't have enough depth to satisfy the
// threshold. This is the key used by the shared-directory bucket.
func dirOf(p string, depth int) string {
	d := path.Dir(p)
	parts := strings.Split(d, "/")
	if len(parts) < depth {
		return ""
	}
	return strings.Join(parts[:depth], "/")
}

func clusterSignature(members map[string]bool) string {
	keys := make([]string, 0, len(members))
	for k := range members {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "|")
}

func pickAnchor(members []kleio.RawSignal) string {
	bestScore := -1
	best := ""
	for _, m := range members {
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
			best = signalKey(m)
		}
	}
	if best == "" && len(members) > 0 {
		best = signalKey(members[0])
	}
	return best
}

func anchorType(members []kleio.RawSignal, anchor string) string {
	for _, m := range members {
		if signalKey(m) == anchor {
			return m.SourceType
		}
	}
	if len(members) > 0 {
		return members[0].SourceType
	}
	return ""
}

func signalKey(s kleio.RawSignal) string {
	if s.SourceID != "" {
		return s.SourceID
	}
	return s.SourceType + ":" + s.SourceOffset
}
