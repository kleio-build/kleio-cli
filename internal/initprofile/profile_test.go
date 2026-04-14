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
	require.Equal(t, []ID{Cursor, Claude, Windsurf, Copilot, Codex}, got)

	_, err = ParseList("nope")
	require.Error(t, err)
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
