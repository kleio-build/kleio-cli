package commands

import (
	"fmt"
	"strings"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/config"
	"github.com/kleio-build/kleio-cli/internal/mcpdetect"
)

func printReadiness(cfg *config.Config) {
	fmt.Println()
	fmt.Println("Readiness")
	api := strings.TrimSpace(cfg.APIURL)
	if api == "" {
		fmt.Println("  api_url:   (empty)")
	} else {
		fmt.Printf("  api_url:   OK (%s)\n", api)
	}
	switch {
	case !config.HasAuth(cfg):
		fmt.Println("  auth:      missing (no OAuth token, no API key)")
	case config.UsingOAuth(cfg):
		fmt.Println("  auth:      OK (OAuth)")
	default:
		fmt.Println("  auth:      OK (API key)")
	}
	if !config.HasWorkspace(cfg) {
		fmt.Println("  workspace: missing")
	} else {
		ws := strings.TrimSpace(cfg.WorkspaceID)
		if len(ws) > 12 {
			ws = ws[:12] + "…"
		}
		fmt.Printf("  workspace: OK (%s)\n", ws)
	}
}

// RunInitVerify performs a lightweight API check (health + auth). Returns an error if verification fails.
func RunInitVerify(getClient func() *client.Client) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !config.HasAuth(cfg) || !config.HasWorkspace(cfg) {
		return fmt.Errorf("missing auth or workspace")
	}
	apiURL := strings.TrimSpace(cfg.APIURL)
	if apiURL == "" {
		return fmt.Errorf("api_url is empty")
	}
	if err := client.HealthCheck(apiURL, nil); err != nil {
		return fmt.Errorf("API health: %w", err)
	}
	cl := getClient()
	if _, err := cl.GetMeJSON(); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	return nil
}

func printInitVerify(ok bool, detail string) {
	fmt.Println()
	fmt.Println("Init verify")
	if ok {
		fmt.Println("  API:       OK")
		if detail != "" {
			fmt.Printf("  %s\n", detail)
		}
		return
	}
	fmt.Printf("  API:       failed (%s)\n", detail)
}

func printNextSteps(wantsCursor, wantsClaude, wantsGeneric bool, writtenDests []string, projectDir string, verifyOK bool) {
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println(`  • Track your first slice: kleio checkpoint "Kleio CLI ready" --slice-category implementation --slice-status completed --validation-status passed`)

	if wantsCursor || installedCursorArtifacts(writtenDests) {
		printCursorMCPNextSteps(projectDir)
	}
	if wantsClaude || installedClaudeArtifacts(writtenDests) {
		printClaudeCodeNextSteps(writtenDests)
	}
	if wantsGeneric && !wantsCursor && !wantsClaude {
		fmt.Println("  • Editor-agnostic: follow AGENTS.md (or AGENTS.kleio.md) and run `kleio` from your terminal; add Kleio to your editor later with `kleio init --tool=cursor` or `--tool=claude` if needed.")
	}

	if !verifyOK {
		fmt.Println("  • Diagnose: kleio status")
	}
}

func printCursorMCPNextSteps(projectDir string) {
	mcp := mcpdetect.Scan(projectDir)
	fmt.Println("  • Cursor (MCP + agent):")
	if !mcp.Configured {
		fmt.Println("    - Merge .cursor/mcp.json.example or .cursor/mcp.kleio.json.example into your Cursor MCP config (project .cursor/mcp.json or user ~/.cursor/mcp.json); use command kleio, args [mcp], and env for KLEIO_API_URL / KLEIO_API_KEY or token as needed.")
	} else {
		fmt.Println("    - Kleio already appears in a Cursor MCP config; re-merge the example if you change API URL or workspace.")
	}
	fmt.Println("    - Fully quit and restart Cursor so it reloads MCP servers (not just reload window).")
	fmt.Println("    - Verify: run `kleio status` and check the MCP section lists Kleio in project or user config.")
	fmt.Println("    - Test with the agent: in Chat, ask the AI to use a Kleio MCP tool (e.g. `kleio_backlog_list` or `kleio_checkpoint`) and confirm it responds — that proves the MCP server is running.")
}

func printClaudeCodeNextSteps(writtenDests []string) {
	hasSidecar := false
	for _, p := range writtenDests {
		if strings.Contains(filepathToSlash(p), "CLAUDE.kleio.yaml") {
			hasSidecar = true
			break
		}
	}
	fmt.Println("  • Claude Code:")
	fmt.Println("    - Use `CLAUDE.md` (or merge `CLAUDE.kleio.yaml` if that was installed) so Claude Code picks up Kleio guidance.")
	if hasSidecar {
		fmt.Println("    - You declined to overwrite an existing `CLAUDE.md`; open `CLAUDE.kleio.yaml` and merge its notes into your preferred Claude config or project docs.")
	}
	fmt.Println("    - Run `kleio` from the Claude Code terminal (or your system shell) for checkpoints and captures; there is no separate Kleio MCP transport for Claude Code today.")
}

func installedClaudeArtifacts(writtenDests []string) bool {
	for _, p := range writtenDests {
		s := filepathToSlash(p)
		if strings.Contains(s, "CLAUDE.md") || strings.Contains(s, "CLAUDE.kleio.yaml") {
			return true
		}
	}
	return false
}

func installedCursorArtifacts(writtenDests []string) bool {
	for _, p := range writtenDests {
		s := filepathToSlash(p)
		if strings.Contains(s, ".cursor/") && strings.Contains(s, "mcp") {
			return true
		}
	}
	return false
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
