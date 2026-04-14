package initprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ID names editor/tooling bundles for kleio init.
type ID string

const (
	Cursor  ID = "cursor"
	Claude  ID = "claude"
	Generic ID = "generic"
	None    ID = "none"
	All     ID = "all"
)

// Embed paths are relative to the embedded templates root (see bootstrap.TemplateFS).
var (
	// CommonFiles are included for any profile that installs project files (except none).
	CommonFiles = []string{
		"AGENTS.md",
	}

	// Cursor files use the `cursor/` prefix in the embed FS because Go embed omits dot-directories.
	// EmbedToDestRel maps them to `.cursor/...` in the target repo.
	CursorFiles = []string{
		"kleio.config.example.yaml",
		"cursor/rules/kleio-mcp.mdc",
		"cursor/skills/kleio-checkpoint-logging/SKILL.md",
		"cursor/skills/kleio-decision-logging/SKILL.md",
		"cursor/mcp.json.example",
		"cursor/hooks.json",
	}

	ClaudeFiles = []string{
		"CLAUDE.md",
	}
)

// FilesFor returns template paths for a profile id.
func FilesFor(id ID) ([]string, error) {
	switch id {
	case None:
		return nil, nil
	case Generic:
		return append([]string{}, CommonFiles...), nil
	case Claude:
		out := append(append([]string{}, CommonFiles...), ClaudeFiles...)
		return uniq(out), nil
	case Cursor:
		out := append(append([]string{}, CommonFiles...), CursorFiles...)
		return uniq(out), nil
	case All:
		out := append(append(append([]string{}, CommonFiles...), CursorFiles...), ClaudeFiles...)
		return uniq(out), nil
	default:
		return nil, fmt.Errorf("unknown profile: %s", id)
	}
}

// ParseList parses comma-separated profile ids (e.g. "cursor,claude").
func ParseList(s string) ([]ID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty --tool")
	}
	parts := strings.Split(s, ",")
	var out []ID
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			continue
		}
		id := ID(p)
		switch id {
		case Cursor, Claude, Generic, None, All:
			out = append(out, id)
		default:
			return nil, fmt.Errorf("unknown profile %q (valid: cursor, claude, generic, none, all)", p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no profiles in --tool")
	}
	return ExpandAll(out), nil
}

// ExpandAll replaces "all" with cursor+claude (generic is implied via file union).
func ExpandAll(ids []ID) []ID {
	hasAll := false
	for _, id := range ids {
		if id == All {
			hasAll = true
			break
		}
	}
	if hasAll {
		return []ID{Cursor, Claude}
	}
	return uniqID(ids)
}

// MergeProfiles returns the union of files for multiple profiles.
func MergeProfiles(ids []ID) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	for _, id := range ids {
		if id == None {
			continue
		}
		files, err := FilesFor(id)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if _, ok := seen[f]; ok {
				continue
			}
			seen[f] = struct{}{}
			out = append(out, f)
		}
	}
	return out, nil
}

func uniq(ss []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range ss {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func uniqID(ids []ID) []ID {
	seen := map[ID]struct{}{}
	var out []ID
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// EmbedToDestRel maps an embedded template path to the path written in the project tree.
func EmbedToDestRel(embedRel string) string {
	embedRel = filepath.ToSlash(embedRel)
	if strings.HasPrefix(embedRel, "cursor/") {
		return ".cursor/" + strings.TrimPrefix(embedRel, "cursor/")
	}
	return embedRel
}

// SidecarPath returns the kleio sidecar path when the user declines overwrite.
// rel must be the destination-relative path (e.g. `.cursor/mcp.json.example`), not the embed path.
func SidecarPath(rel string) string {
	rel = filepath.ToSlash(rel)
	base := filepath.Base(rel)
	dir := filepath.Dir(rel)
	switch {
	case base == "AGENTS.md":
		return filepath.Join(dir, "AGENTS.kleio.md")
	case base == "kleio.config.example.yaml":
		return filepath.Join(dir, "kleio.config.kleio.example.yaml")
	case base == "mcp.json.example" && strings.HasPrefix(rel, ".cursor/"):
		return filepath.Join(dir, "mcp.kleio.json.example")
	case base == "CLAUDE.md":
		return filepath.Join(dir, "CLAUDE.kleio.yaml")
	default:
		if dir == "." {
			return "kleio." + base
		}
		return filepath.Join(dir, "kleio."+base)
	}
}

// Recommend returns a suggested profile from repo signals.
func Recommend(root string) ID {
	signals := DetectSignals(root)
	for _, s := range signals {
		if s == ".cursor/" {
			return Cursor
		}
	}
	for _, s := range signals {
		if s == ".claude/" || s == "CLAUDE.md" {
			return Claude
		}
	}
	return Cursor
}

// DetectSignals mirrors init detection (dirs + marker files).
func DetectSignals(root string) []string {
	var out []string
	candidates := []string{".cursor", ".claude", ".github"}
	for _, p := range candidates {
		if st, err := statDir(root, p); err == nil && st {
			out = append(out, p+"/")
		}
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"} {
		if existsFile(root, name) {
			out = append(out, name)
		}
	}
	return out
}

func statDir(root, name string) (bool, error) {
	st, err := os.Stat(filepath.Join(root, name))
	if err != nil {
		return false, err
	}
	return st.IsDir(), nil
}

func existsFile(root, name string) bool {
	st, err := os.Stat(filepath.Join(root, name))
	if err != nil {
		return false
	}
	return !st.IsDir()
}
