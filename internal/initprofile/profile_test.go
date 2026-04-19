package initprofile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbedToDestRel(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"cursor/hooks.json", ".cursor/hooks.json"},
		{"claude/settings.json", ".claude/settings.json"},
		{"windsurf/hooks.json", ".windsurf/hooks.json"},
		{"github/hooks/kleio-hooks.json", ".github/hooks/kleio-hooks.json"},
		{"codex/hooks.json", ".codex/hooks.json"},
		{"opencode/opencode.json.example", "opencode.json.example"},
		{"opencode/opencode.http.json.example", "opencode.http.json.example"},
		{"opencode/AGENTS.opencode.md", "AGENTS.opencode.md"},
		{"opencode/hooks/kleio-auth-check.sh", ".opencode/hooks/kleio-auth-check.sh"},
		{"AGENTS.md", "AGENTS.md"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.want, EmbedToDestRel(tc.in))
		})
	}
}

func TestParseList_andExpandAll(t *testing.T) {
	got, err := ParseList("cursor,claude")
	require.NoError(t, err)
	require.Equal(t, []ID{Cursor, Claude}, got)

	got, err = ParseList("windsurf")
	require.NoError(t, err)
	require.Equal(t, []ID{Windsurf}, got)

	got, err = ParseList("all")
	require.NoError(t, err)
	require.Equal(t, []ID{Cursor, Claude, Windsurf, Copilot, Codex, OpenCode}, got)

	got, err = ParseList("opencode")
	require.NoError(t, err)
	require.Equal(t, []ID{OpenCode}, got)

	_, err = ParseList("nope")
	require.Error(t, err)
}

// SC-OC1 / SC-OC3 — opencode profile installs the canonical bootstrap files.
func TestFilesFor_OpenCode(t *testing.T) {
	files, err := FilesFor(OpenCode)
	require.NoError(t, err)
	require.Contains(t, files, "AGENTS.md")
	require.Contains(t, files, "opencode/opencode.json.example")
	require.Contains(t, files, "opencode/opencode.http.json.example")
	require.Contains(t, files, "opencode/hooks/kleio-auth-check.sh")
	require.Contains(t, files, "opencode/AGENTS.opencode.md")
}

func TestDetectSignals_OpenCode(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "opencode.json"), []byte("{}"), 0644))
	sig := DetectSignals(root)
	require.Contains(t, sig, "opencode.json")
	require.Equal(t, OpenCode, Recommend(root))
}

func TestFilesFor_All(t *testing.T) {
	files, err := FilesFor(All)
	require.NoError(t, err)
	require.Contains(t, files, "AGENTS.md")
	require.Contains(t, files, "cursor/hooks.json")
	require.Contains(t, files, "claude/settings.json")
	require.Contains(t, files, "windsurf/hooks.json")
	require.Contains(t, files, "github/hooks/kleio-hooks.json")
	require.Contains(t, files, "codex/hooks.json")
	require.Contains(t, files, "opencode/opencode.json.example")
}

func TestSidecarPath_claudeAndWindsurf(t *testing.T) {
	require.Equal(t, filepath.Join(".claude", "settings.kleio.json"), SidecarPath(".claude/settings.json"))
	require.Equal(t, filepath.Join(".windsurf", "kleio.hooks.json"), SidecarPath(".windsurf/hooks.json"))
}

func TestDetectSignals_andRecommend(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".windsurf"), 0755))
	sig := DetectSignals(root)
	require.Contains(t, sig, ".windsurf/")
	require.Equal(t, Windsurf, Recommend(root))

	require.NoError(t, os.Mkdir(filepath.Join(root, ".cursor"), 0755))
	require.Equal(t, Cursor, Recommend(root))
}
