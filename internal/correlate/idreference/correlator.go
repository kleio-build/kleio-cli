// Package idreference implements a kleio.Correlator that groups signals
// sharing an explicit identifier reference: KL-N backlog ids, plan
// filename hashes (the trailing 8-hex suffix on .plan.md files), GitHub
// PR refs (#NNN), or commit SHAs.
//
// This is the highest-confidence correlator we have because each match
// represents the author explicitly linking signals together. Time-based
// or text-based correlators are softer signals; ID matches almost
// always reflect ground-truth intent.
package idreference

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	kleio "github.com/kleio-build/kleio-core"
)

// Correlator implements kleio.Correlator. It scans every signal's
// Content (plus key Metadata fields) for known ID patterns and emits
// one Cluster per distinct ID. A signal can appear in multiple
// clusters when it references multiple IDs.
type Correlator struct {
	// MinClusterSize gates emission. Default 2 means an ID seen in only
	// one signal yields no cluster.
	MinClusterSize int
}

func New() *Correlator { return &Correlator{MinClusterSize: 2} }

func (c *Correlator) Name() string { return "id_reference" }

// Reference patterns. Anchored to word boundaries so we don't match
// substrings of unrelated identifiers.
var (
	klRefRe        = regexp.MustCompile(`\b(KL-\d+)\b`)
	prRefRe        = regexp.MustCompile(`\B#(\d{1,5})\b`)
	planFilenameRe = regexp.MustCompile(`([a-z0-9_-]+_[0-9a-f]{8})\.plan\.md`)
	planAnchorRe   = regexp.MustCompile(`\b([a-z][a-z0-9_-]+_[0-9a-f]{8})\b`)
	shaRe          = regexp.MustCompile(`\b([0-9a-f]{7,40})\b`)
)

// Correlate scans all signals once, building an inverted index from ID
// -> []signal, then emits clusters for IDs with at least
// MinClusterSize members. The anchor of each cluster is the first
// signal that mentions the ID in its SourceID/SourceOffset (i.e. is
// the ID's source-of-truth) when one exists; otherwise the earliest
// signal by Timestamp wins.
func (c *Correlator) Correlate(ctx context.Context, signals []kleio.RawSignal) ([]kleio.Cluster, error) {
	minSize := c.MinClusterSize
	if minSize <= 0 {
		minSize = 2
	}

	type ref struct {
		idType string // "KL", "PR", "plan", "sha"
		id     string
	}

	index := map[ref]map[string]bool{}     // ref -> set(SourceID)
	signalsByKey := map[string]kleio.RawSignal{}

	for _, s := range signals {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		key := signalKey(s)
		signalsByKey[key] = s

		haystacks := []string{s.Content, s.SourceID, s.SourceOffset}
		if filePath, ok := s.Metadata["file_path"].(string); ok {
			haystacks = append(haystacks, filePath)
		}
		if planFile, ok := s.Metadata["plan_file"].(string); ok {
			haystacks = append(haystacks, planFile)
		}
		body := strings.Join(haystacks, "\n")

		for _, m := range klRefRe.FindAllStringSubmatch(body, -1) {
			r := ref{idType: "KL", id: m[1]}
			if index[r] == nil {
				index[r] = map[string]bool{}
			}
			index[r][key] = true
		}
		for _, m := range prRefRe.FindAllStringSubmatch(body, -1) {
			r := ref{idType: "PR", id: "#" + m[1]}
			if index[r] == nil {
				index[r] = map[string]bool{}
			}
			index[r][key] = true
		}
		for _, m := range planFilenameRe.FindAllStringSubmatch(body, -1) {
			r := ref{idType: "plan", id: m[1]}
			if index[r] == nil {
				index[r] = map[string]bool{}
			}
			index[r][key] = true
		}
		// Plan anchors (e.g. report_quality_fixes_de202626) without the
		// .plan.md suffix - common in transcript prose and commit
		// messages. We only accept hashes that look like 8 hex chars
		// preceded by an underscore so we don't match arbitrary
		// snake_case_words.
		for _, m := range planAnchorRe.FindAllStringSubmatch(body, -1) {
			r := ref{idType: "plan", id: m[1]}
			if _, alreadyIndexed := index[r]; alreadyIndexed {
				index[r][key] = true
				continue
			}
			if index[r] == nil {
				index[r] = map[string]bool{}
			}
			index[r][key] = true
		}
		// Commit SHAs only when the host signal isn't itself a commit
		// (otherwise every commit clusters with itself). We scan ONLY
		// the Content field for SHAs -- transcripts and plans have
		// hex-shaped UUIDs in their SourceIDs (e.g.
		// "9fcacd77-7c0c-45db-a714-ce8f31124434") that would
		// false-positive as commit SHA refs every time.
		if s.SourceType != kleio.SourceTypeLocalGit {
			for _, m := range shaRe.FindAllStringSubmatch(s.Content, -1) {
				if len(m[1]) < 7 {
					continue
				}
				r := ref{idType: "sha", id: m[1]}
				if index[r] == nil {
					index[r] = map[string]bool{}
				}
				index[r][key] = true
			}
		}
	}

	// Cross-link commits to SHA references: if any commit's SourceID
	// matches a SHA reference we collected, add the commit signal to
	// that cluster.
	for _, s := range signals {
		if s.SourceType != kleio.SourceTypeLocalGit {
			continue
		}
		sha, ok := s.Metadata["sha"].(string)
		if !ok || sha == "" {
			continue
		}
		key := signalKey(s)
		signalsByKey[key] = s
		for r := range index {
			if r.idType != "sha" {
				continue
			}
			if strings.HasPrefix(sha, r.id) || strings.HasPrefix(r.id, sha[:min(len(sha), len(r.id))]) {
				index[r][key] = true
			}
		}
	}

	refs := make([]ref, 0, len(index))
	for r := range index {
		refs = append(refs, r)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].idType != refs[j].idType {
			return refs[i].idType < refs[j].idType
		}
		return refs[i].id < refs[j].id
	})

	var clusters []kleio.Cluster
	for _, r := range refs {
		members := index[r]
		if len(members) < minSize {
			continue
		}
		memberSlice := make([]kleio.RawSignal, 0, len(members))
		for k := range members {
			memberSlice = append(memberSlice, signalsByKey[k])
		}

		// SHA-based clusters are only meaningful when at least one
		// member is an actual git commit. Otherwise we're matching on
		// the random hex suffix of unrelated transcript UUIDs (which
		// share a 32-char hex namespace with SHAs and produce
		// thousands of spurious matches).
		if r.idType == "sha" {
			hasCommit := false
			for _, m := range memberSlice {
				if m.SourceType == kleio.SourceTypeLocalGit {
					hasCommit = true
					break
				}
			}
			if !hasCommit {
				continue
			}
		}
		sort.Slice(memberSlice, func(i, j int) bool {
			return memberSlice[i].Timestamp.Before(memberSlice[j].Timestamp)
		})

		anchor := pickIDAnchor(memberSlice, r)
		links := make([]kleio.ClusterLink, 0, len(memberSlice)-1)
		for _, m := range memberSlice {
			if signalKey(m) == anchor {
				continue
			}
			links = append(links, kleio.ClusterLink{
				From:       anchor,
				To:         signalKey(m),
				LinkType:   kleio.LinkTypeReferences,
				Confidence: 0.85,
				Reason:     fmt.Sprintf("%s_ref=%s", r.idType, r.id),
			})
		}
		clusters = append(clusters, kleio.Cluster{
			AnchorID:   anchor,
			AnchorType: anchorTypeFor(memberSlice, anchor),
			Members:    memberSlice,
			Links:      links,
			Confidence: 0.85,
			Provenance: []string{fmt.Sprintf("id_reference:%s=%s", r.idType, r.id)},
		})
	}
	return clusters, nil
}

// pickIDAnchor: a plan whose filename matches the ref is the canonical
// anchor; otherwise the earliest signal wins. This matches the user's
// stated intent that plans are the authoritative record of work.
func pickIDAnchor(members []kleio.RawSignal, r struct {
	idType string
	id     string
}) string {
	if r.idType == "plan" {
		for _, m := range members {
			if m.SourceType == "cursor_plan" && strings.Contains(m.SourceID, r.id) {
				return signalKey(m)
			}
		}
	}
	if r.idType == "sha" {
		for _, m := range members {
			if m.SourceType == kleio.SourceTypeLocalGit {
				if sha, ok := m.Metadata["sha"].(string); ok && strings.HasPrefix(sha, r.id) {
					return signalKey(m)
				}
			}
		}
	}
	return signalKey(members[0])
}

func anchorTypeFor(members []kleio.RawSignal, anchor string) string {
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
