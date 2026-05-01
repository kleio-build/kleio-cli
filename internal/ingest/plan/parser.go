// Package plan implements a kleio.Ingester that extracts RawSignals from
// .cursor/plans/*.plan.md files. Plans are the most structured artifact
// the user produces: each contains an umbrella goal, a list of todos with
// statuses, decision blocks, out-of-scope sections, and risks. The
// PlanIngester runs five passes over each plan to lift each into a
// RawSignal that the correlation/synthesis layers can group and reduce.
package plan

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"gopkg.in/yaml.v3"
)

// PlanFrontmatter is the structured YAML at the top of every plan file.
// Only fields the ingester reads are unmarshaled; unknown keys are
// preserved by the YAML parser as a no-op.
type PlanFrontmatter struct {
	Name      string `yaml:"name"`
	Overview  string `yaml:"overview"`
	IsProject bool   `yaml:"isProject"`
	Todos     []struct {
		ID      string `yaml:"id"`
		Content string `yaml:"content"`
		Status  string `yaml:"status"`
	} `yaml:"todos"`
}

// ParsedPlan holds everything extracted from a single .plan.md file.
type ParsedPlan struct {
	Path        string
	Frontmatter PlanFrontmatter
	Body        string
	BodyOffset  int // line number where the body begins (after closing ---)
	ModTime     time.Time
}

// signalEmitter is shared state across all five passes; each pass appends
// directly into the slice. Centralising it lets us deduplicate and order
// signals before returning.
type signalEmitter struct {
	signals []kleio.RawSignal
}

func (e *signalEmitter) emit(s kleio.RawSignal) { e.signals = append(e.signals, s) }

// ParseFile reads a .plan.md file and returns the structured ParsedPlan.
// Errors are returned for file IO failures; YAML parse errors degrade
// gracefully (empty Frontmatter, full body still available for body-pass
// extraction).
func ParseFile(path string) (*ParsedPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	st, _ := os.Stat(path)
	pp := &ParsedPlan{Path: path}
	if st != nil {
		pp.ModTime = st.ModTime()
	}
	pp.Frontmatter, pp.Body, pp.BodyOffset = splitFrontmatter(string(data))
	return pp, nil
}

// splitFrontmatter extracts the YAML between the first two `---` markers.
// Returns the parsed frontmatter (zero-valued on error), the body string,
// and the 1-indexed line number where the body begins.
func splitFrontmatter(content string) (PlanFrontmatter, string, int) {
	var fm PlanFrontmatter
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return fm, content, 1
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return fm, content, 1
	}
	yamlText := strings.Join(lines[1:end], "\n")
	_ = yaml.Unmarshal([]byte(yamlText), &fm) // tolerate parse errors
	body := strings.Join(lines[end+1:], "\n")
	return fm, body, end + 2
}

// SignalsFromPlan runs every pass over pp and returns a deterministic,
// deduplicated slice of RawSignals. Passes:
//   1. Frontmatter todos
//   2. Decision blocks
//   3. Out-of-scope / deferred sections
//   4. Risks / open questions
//   5. Umbrella plan signal (one per file)
func SignalsFromPlan(pp *ParsedPlan, repoName string) []kleio.RawSignal {
	e := &signalEmitter{}
	planID := planSourceID(pp.Path)

	for _, t := range pp.Frontmatter.Todos {
		if strings.TrimSpace(t.Content) == "" {
			continue
		}
		e.emit(kleio.RawSignal{
			SourceType:   "cursor_plan",
			SourceID:     planID + "#todo:" + t.ID,
			SourceOffset: "todo:" + t.ID,
			Content:      strings.TrimSpace(t.Content),
			Kind:         kleio.SignalTypeWorkItem,
			Timestamp:    pp.ModTime,
			RepoName:     repoName,
			Metadata: map[string]any{
				"plan_path":      pp.Path,
				"plan_name":      pp.Frontmatter.Name,
				"plan_anchor_id": planID,
				"todo_id":        t.ID,
				"status":         t.Status,
			},
		})
	}

	extractDecisions(pp, planID, repoName, e)
	extractDeferred(pp, planID, repoName, e)
	extractRisks(pp, planID, repoName, e)
	extractUmbrella(pp, planID, repoName, e)

	return e.signals
}

// planSourceID returns a stable ID for a plan derived from its filename
// (basename without extension). The trailing 8-char hash in plan filenames
// is already unique across the repo, so basename is enough.
func planSourceID(path string) string {
	return "plan:" + strings.TrimSuffix(filepath.Base(path), ".plan.md")
}

var (
	headingRe       = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)\s*$`)
	deferredHeading = regexp.MustCompile(`(?i)^(out of scope|future enhancements?|deferred|later|won't (do|build|tackle|fix)|will not)`)
	risksHeading    = regexp.MustCompile(`(?i)^(risks?|open questions?|trade[- ]?offs?|unknowns?)\b`)
	decisionInline  = regexp.MustCompile(`(?im)^(?:\*\*Decision[:*]?\*\*|###\s+Decision\b|^Decision:)\s*(.*)$`)
	bulletRe        = regexp.MustCompile(`^\s*[-*+]\s+(.+?)\s*$`)
	rationaleRe     = regexp.MustCompile(`(?im)^\s*(?:rationale|because|why)[:\-]\s*(.+)$`)
)

// extractDecisions emits one RawSignal per Decision block found in the
// body. A "block" is the line matching decisionInline plus the next
// non-blank line of rationale (when present).
func extractDecisions(pp *ParsedPlan, planID, repoName string, e *signalEmitter) {
	lines := strings.Split(pp.Body, "\n")
	for i, line := range lines {
		m := decisionInline.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		content := strings.TrimSpace(m[1])
		var rationale string
		// Look ahead up to 3 lines for an inline rationale.
		for j := i + 1; j < len(lines) && j <= i+3; j++ {
			if rm := rationaleRe.FindStringSubmatch(lines[j]); rm != nil {
				rationale = strings.TrimSpace(rm[1])
				break
			}
			if strings.TrimSpace(lines[j]) == "" && rationale != "" {
				break
			}
		}
		if content == "" {
			content = strings.TrimSpace(line)
		}
		if len(strings.Fields(content)) < 2 {
			continue
		}
		md := map[string]any{
			"plan_path":      pp.Path,
			"plan_name":      pp.Frontmatter.Name,
			"plan_anchor_id": planID,
		}
		if rationale != "" {
			md["rationale"] = rationale
		}
		e.emit(kleio.RawSignal{
			SourceType:   "cursor_plan",
			SourceID:     fmt.Sprintf("%s#decision:L%d", planID, pp.BodyOffset+i),
			SourceOffset: fmt.Sprintf("decision:L%d", pp.BodyOffset+i),
			Content:      content,
			Kind:         kleio.SignalTypeDecision,
			Timestamp:    pp.ModTime,
			RepoName:     repoName,
			Metadata:     md,
		})
	}
}

// extractDeferred walks the body and, for every heading matching the
// "deferred" classifier, emits one RawSignal per bullet underneath it.
// Headings stop at the next heading of equal-or-higher level.
func extractDeferred(pp *ParsedPlan, planID, repoName string, e *signalEmitter) {
	emitBulletsUnderHeadings(pp, planID, repoName, e, deferredHeading, "deferred", kleio.SignalTypeWorkItem, true)
}

// extractRisks does the same for risk/open-questions/trade-offs sections.
func extractRisks(pp *ParsedPlan, planID, repoName string, e *signalEmitter) {
	emitBulletsUnderHeadings(pp, planID, repoName, e, risksHeading, "risk", kleio.SignalTypeWorkItem, false)
}

// emitBulletsUnderHeadings is the shared loop for deferred/risk passes.
// It scans `pp.Body` line-by-line; whenever a heading matches headingClassifier,
// every bullet (`-`, `*`, `+`) inside that section becomes a RawSignal.
func emitBulletsUnderHeadings(
	pp *ParsedPlan,
	planID, repoName string,
	e *signalEmitter,
	headingClassifier *regexp.Regexp,
	offsetPrefix string,
	kind string,
	deferredFlag bool,
) {
	lines := strings.Split(pp.Body, "\n")
	insideLevel := -1 // -1 == not inside a relevant section
	scanner := bufio.NewScanner(strings.NewReader(pp.Body))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for i, line := range lines {
		if hm := headingRe.FindStringSubmatch(line); hm != nil {
			level := len(hm[1])
			heading := strings.TrimSpace(hm[2])
			if headingClassifier.MatchString(heading) {
				insideLevel = level
				continue
			}
			// New heading at same-or-lower-level closes the section.
			if insideLevel != -1 && level <= insideLevel {
				insideLevel = -1
			}
			continue
		}
		if insideLevel == -1 {
			continue
		}
		bm := bulletRe.FindStringSubmatch(line)
		if bm == nil {
			continue
		}
		content := strings.TrimSpace(bm[1])
		if content == "" {
			continue
		}
		md := map[string]any{
			"plan_path":      pp.Path,
			"plan_name":      pp.Frontmatter.Name,
			"plan_anchor_id": planID,
		}
		if deferredFlag {
			md["deferred"] = true
		}
		e.emit(kleio.RawSignal{
			SourceType:   "cursor_plan",
			SourceID:     fmt.Sprintf("%s#%s:L%d", planID, offsetPrefix, pp.BodyOffset+i),
			SourceOffset: fmt.Sprintf("%s:L%d", offsetPrefix, pp.BodyOffset+i),
			Content:      content,
			Kind:         kind,
			Timestamp:    pp.ModTime,
			RepoName:     repoName,
			Metadata:     md,
		})
	}
}

// extractUmbrella emits exactly one RawSignal per plan, marked as
// is_anchor=true so correlators recognise it as the canonical anchor for
// the plan-cluster.
func extractUmbrella(pp *ParsedPlan, planID, repoName string, e *signalEmitter) {
	content := pp.Frontmatter.Name
	if content == "" {
		content = strings.TrimSuffix(filepath.Base(pp.Path), ".plan.md")
	}
	if pp.Frontmatter.Overview != "" {
		content = content + " - " + pp.Frontmatter.Overview
	}
	e.emit(kleio.RawSignal{
		SourceType:   "cursor_plan",
		SourceID:     planID,
		SourceOffset: "umbrella",
		Content:      content,
		Kind:         kleio.SignalTypeCheckpoint,
		Timestamp:    pp.ModTime,
		RepoName:     repoName,
		Metadata: map[string]any{
			"plan_path":      pp.Path,
			"plan_name":      pp.Frontmatter.Name,
			"plan_anchor_id": planID,
			"is_anchor":      true,
			"is_project":     pp.Frontmatter.IsProject,
		},
	})
}
