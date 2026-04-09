package bootstrap

import (
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
