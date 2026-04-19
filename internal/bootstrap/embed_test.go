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

// SC-OC1 — opencode.json template embedded as valid JSON with a Kleio MCP entry.
func TestTemplateFS_opencodeJSON(t *testing.T) {
	fsys, err := TemplateFS()
	require.NoError(t, err)
	b, err := fs.ReadFile(fsys, "opencode/opencode.json.example")
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(b, &parsed))
	mcp, ok := parsed["mcp"].(map[string]any)
	require.True(t, ok, "opencode.json.example must declare mcp{}")
	_, ok = mcp["kleio"]
	require.True(t, ok, "opencode.json.example must declare an mcp.kleio entry")
}

// SC-OC2 — HTTP variant with bearer-token + workspace-id placeholders.
func TestTemplateFS_opencodeHTTPJSON(t *testing.T) {
	fsys, err := TemplateFS()
	require.NoError(t, err)
	b, err := fs.ReadFile(fsys, "opencode/opencode.http.json.example")
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(b, &parsed))
	require.Contains(t, string(b), "X-Workspace-ID")
	require.Contains(t, string(b), "Authorization")
}

func TestTemplateFS_opencodeAuthCheckShebang(t *testing.T) {
	fsys, err := TemplateFS()
	require.NoError(t, err)
	b, err := fs.ReadFile(fsys, "opencode/hooks/kleio-auth-check.sh")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(b), 2)
	require.Equal(t, "#!", string(b[:2]))
}

func TestTemplateFS_opencodeAGENTS(t *testing.T) {
	fsys, err := TemplateFS()
	require.NoError(t, err)
	b, err := fs.ReadFile(fsys, "opencode/AGENTS.opencode.md")
	require.NoError(t, err)
	require.Contains(t, string(b), "kleio_decide")
	require.Contains(t, string(b), "kleio_checkpoint")
}
