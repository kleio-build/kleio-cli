package bootstrap

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func runBashScript(t *testing.T, embedPath, stdin string) (stderr string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("bash hook scripts")
	}
	fsys, err := TemplateFS()
	require.NoError(t, err)
	body, err := fs.ReadFile(fsys, embedPath)
	require.NoError(t, err)
	dir := t.TempDir()
	name := filepath.Base(embedPath)
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, body, 0755))
	cmd := exec.Command("bash", p)
	cmd.Stdin = strings.NewReader(stdin)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	_ = cmd.Run()
	return errBuf.String()
}

func TestKleioAuthCheck_claude(t *testing.T) {
	se := runBashScript(t, "claude/hooks/kleio-auth-check.sh",
		`{"tool_name":"mcp__user-kleio__x","error":"authentication required"}`)
	require.Contains(t, se, "kleio login")
}

func TestKleioAuthCheck_claude_noMatchWithoutKleio(t *testing.T) {
	se := runBashScript(t, "claude/hooks/kleio-auth-check.sh",
		`{"tool_name":"Bash","error":"authentication required"}`)
	require.NotContains(t, se, "kleio login")
}

func TestKleioAuthCheck_windsurf(t *testing.T) {
	se := runBashScript(t, "windsurf/hooks/kleio-auth-check.sh",
		`{"tool_info":{"mcp_server_name":"kleio","mcp_result":"401 unauthorized"}}`)
	require.Contains(t, se, "kleio login")
}

func TestKleioAuthCheck_github(t *testing.T) {
	se := runBashScript(t, "github/hooks/kleio-auth-check.sh",
		`{"toolName":"mcp","error":"401 kleio"}`)
	require.Contains(t, se, "kleio login")
}
