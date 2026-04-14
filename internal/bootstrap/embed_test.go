package bootstrap

import (
	"encoding/json"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTemplateFS_hasAGENTS(t *testing.T) {
	fsys, err := TemplateFS()
	require.NoError(t, err)
	b, err := fs.ReadFile(fsys, "AGENTS.md")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(b), 20, "AGENTS.md too short")
}

func TestTemplateFS_cursorRuleEmbedded(t *testing.T) {
	fsys, err := TemplateFS()
	require.NoError(t, err)
	b, err := fs.ReadFile(fsys, "cursor/rules/kleio-mcp.mdc")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(b), 10, "cursor/rules/kleio-mcp.mdc too short")
}

func TestTemplateFS_claudeSettingsEmbedded(t *testing.T) {
	fsys, err := TemplateFS()
	require.NoError(t, err)
	b, err := fs.ReadFile(fsys, "claude/settings.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &map[string]any{}))
}

func TestTemplateFS_claudeAuthCheckShebang(t *testing.T) {
	fsys, err := TemplateFS()
	require.NoError(t, err)
	b, err := fs.ReadFile(fsys, "claude/hooks/kleio-auth-check.sh")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(b), 2)
	require.Equal(t, "#!", string(b[:2]))
}

func TestTemplateFS_windsurfHooksJSON(t *testing.T) {
	fsys, err := TemplateFS()
	require.NoError(t, err)
	b, err := fs.ReadFile(fsys, "windsurf/hooks.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &map[string]any{}))
}

func TestTemplateFS_copilotHooksJSON(t *testing.T) {
	fsys, err := TemplateFS()
	require.NoError(t, err)
	b, err := fs.ReadFile(fsys, "github/hooks/kleio-hooks.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &map[string]any{}))
}

func TestTemplateFS_codexHooksJSON(t *testing.T) {
	fsys, err := TemplateFS()
	require.NoError(t, err)
	b, err := fs.ReadFile(fsys, "codex/hooks.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &map[string]any{}))
}

func TestTemplateFS_cursorHooksMatcher(t *testing.T) {
	fsys, err := TemplateFS()
	require.NoError(t, err)
	b, err := fs.ReadFile(fsys, "cursor/hooks.json")
	require.NoError(t, err)
	require.Contains(t, string(b), "mcp__user-kleio__")
}
