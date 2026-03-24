package mcpdetect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Info is informational only for `kleio status` (never affects exit code).
type Info struct {
	ProjectConfigPath string
	UserConfigPath    string
	Configured        bool // kleio + mcp found in at least one mcp.json
	ProjectHasKleio   bool
	UserHasKleio      bool
	ProcessHint       string // human-readable best-effort
}

// mcpFile mirrors the subset of Cursor MCP config we care about.
type mcpRoot struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func isKleioMCP(e mcpServerEntry) bool {
	cmd := strings.TrimSpace(strings.ToLower(e.Command))
	base := filepath.Base(cmd)
	if base != "kleio" && !strings.HasSuffix(base, "kleio.exe") {
		return false
	}
	for _, a := range e.Args {
		if strings.TrimSpace(strings.ToLower(a)) == "mcp" {
			return true
		}
	}
	return false
}

func scanFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var root mcpRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return false
	}
	for _, e := range root.MCPServers {
		if isKleioMCP(e) {
			return true
		}
	}
	return false
}

// Scan looks for Cursor MCP config with kleio + mcp args.
func Scan(projectDir string) Info {
	home, _ := os.UserHomeDir()
	userPath := filepath.Join(home, ".cursor", "mcp.json")
	projPath := filepath.Join(projectDir, ".cursor", "mcp.json")

	info := Info{
		ProjectConfigPath: projPath,
		UserConfigPath:    userPath,
	}
	info.ProjectHasKleio = scanFile(projPath)
	info.UserHasKleio = scanFile(userPath)
	info.Configured = info.ProjectHasKleio || info.UserHasKleio

	info.ProcessHint = processHint()
	return info
}

func processHint() string {
	// Stdio MCP has no port; best-effort process check only.
	if runtime.GOOS == "windows" {
		return "process check not available on Windows (stdio MCP has no daemon port)"
	}
	// Optional: could use `pgrep -f 'kleio.*mcp'` — skip to avoid shelling out in v1
	return "not detected (normal when Cursor is closed or when using CLI from a terminal)"
}
