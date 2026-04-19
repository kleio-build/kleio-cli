package commands

import (
	"fmt"
	"strings"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/config"
	"github.com/kleio-build/kleio-cli/internal/initprofile"
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

func printNextSteps(ids []initprofile.ID, writtenDests []string, projectDir string, verifyOK bool) {
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println(`  • Track your first slice: kleio checkpoint "Kleio CLI ready" --slice-category implementation --slice-status completed --validation-status passed`)

	wants := func(id initprofile.ID) bool {
		return profileIDsInclude(ids, id)
	}

	if wants(initprofile.Cursor) || installedCursorArtifacts(writtenDests) {
		printCursorMCPNextSteps(projectDir)
	}
	if wants(initprofile.Claude) || installedClaudeArtifacts(writtenDests) {
		printClaudeCodeNextSteps(writtenDests)
	}
	if wants(initprofile.Windsurf) || installedWindsurfArtifacts(writtenDests) {
		printWindsurfNextSteps(writtenDests)
	}
	if wants(initprofile.Copilot) || installedCopilotArtifacts(writtenDests) {
		printCopilotNextSteps()
	}
	if wants(initprofile.Codex) || installedCodexArtifacts(writtenDests) {
		printCodexNextSteps()
	}
	if wants(initprofile.OpenCode) || installedOpenCodeArtifacts(writtenDests) {
		printOpenCodeNextSteps()
	}
	if wants(initprofile.Generic) && !wants(initprofile.Cursor) && !wants(initprofile.Claude) &&
		!wants(initprofile.Windsurf) && !wants(initprofile.Copilot) && !wants(initprofile.Codex) &&
		!wants(initprofile.OpenCode) {
		fmt.Println("  • Editor-agnostic: follow AGENTS.md (or AGENTS.kleio.md) and run `kleio` from your terminal; add Kleio to your editor later with `kleio init --tool=cursor`, `--tool=claude`, etc.")
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
	fmt.Println("    - Merge `.claude/settings.json` (or `settings.kleio.json` if installed as a sidecar) into your Claude Code hooks; ensure `.claude/hooks/kleio-auth-check.sh` is executable (`chmod +x`).")
	fmt.Println("    - Run `kleio` from the Claude Code terminal (or your system shell) for checkpoints and captures, or use Kleio MCP. For cloud agents, use HTTP MCP (`POST /api/mcp` on your Kleio API host) with `Authorization: Bearer` and `X-Workspace-ID`.")
}

func printWindsurfNextSteps(writtenDests []string) {
	fmt.Println("  • Windsurf (Cascade):")
	fmt.Println("    - Hooks live in `.windsurf/hooks.json`; ensure `.windsurf/hooks/kleio-auth-check.sh` is executable (`chmod +x`).")
	hasSidecar := false
	for _, p := range writtenDests {
		if strings.Contains(filepathToSlash(p), "kleio.hooks.json") {
			hasSidecar = true
			break
		}
	}
	if hasSidecar {
		fmt.Println("    - You installed a sidecar hooks file; merge `kleio.hooks.json` into `.windsurf/hooks.json` if Cascade should load Kleio hooks.")
	}
	fmt.Println("    - Configure Kleio MCP in Windsurf per vendor docs; use HTTP MCP for remote/cloud sessions when supported.")
}

func printCopilotNextSteps() {
	fmt.Println("  • GitHub Copilot (cloud agent / CLI):")
	fmt.Println("    - Commit `.github/hooks/kleio-hooks.json` to the default branch for cloud agent hooks; make `.github/hooks/kleio-auth-check.sh` executable.")
	fmt.Println("    - Merge `.github/copilot-instructions.md` into team instructions if you already use a custom file.")
	fmt.Println("    - For remote MCP, point the agent at your Kleio API `POST /api/mcp` with bearer token and `X-Workspace-ID` (see Kleio docs).")
}

func printCodexNextSteps() {
	fmt.Println("  • OpenAI Codex:")
	fmt.Println("    - Enable hooks in `~/.codex/config.toml`: `[features]` then `codex_hooks = true` (experimental; Bash hook events only for MCP today).")
	fmt.Println("    - `.codex/hooks.json` includes SessionStart/Stop reminders; `kleio-session-check.sh` uses `jq`, `python3`, or a tiny grep fallback (no extra install on typical dev machines).")
	fmt.Println("    - Prefer Kleio HTTP MCP in Codex config for cloud or headless runners when supported.")
}

func printOpenCodeNextSteps() {
	fmt.Println("  • OpenCode (sst.dev):")
	fmt.Println("    - Merge `opencode.json.example` (local stdio) or `opencode.http.json.example` (HTTP) into `opencode.json` at your project root or `~/.config/opencode/opencode.json`.")
	fmt.Println("    - Read `AGENTS.opencode.md` for Kleio agent guidance; merge into your `AGENTS.md` once you're happy with the wording.")
	fmt.Println("    - Make `.opencode/hooks/kleio-auth-check.sh` executable (`chmod +x`) if you wire it into a shell pipeline that wraps OpenCode tool output.")
	fmt.Println("    - Verify: run `opencode -p \"call kleio_status\"` (or your preferred non-interactive flag) and confirm Kleio MCP responds.")
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

func installedWindsurfArtifacts(writtenDests []string) bool {
	for _, p := range writtenDests {
		if strings.Contains(filepathToSlash(p), ".windsurf/") {
			return true
		}
	}
	return false
}

func installedCopilotArtifacts(writtenDests []string) bool {
	for _, p := range writtenDests {
		s := filepathToSlash(p)
		if strings.Contains(s, ".github/hooks/kleio") || strings.Contains(s, "copilot-instructions") {
			return true
		}
	}
	return false
}

func installedCodexArtifacts(writtenDests []string) bool {
	for _, p := range writtenDests {
		if strings.Contains(filepathToSlash(p), ".codex/") {
			return true
		}
	}
	return false
}

func installedOpenCodeArtifacts(writtenDests []string) bool {
	for _, p := range writtenDests {
		s := filepathToSlash(p)
		if strings.Contains(s, ".opencode/") || strings.Contains(s, "opencode.json.example") ||
			strings.Contains(s, "AGENTS.opencode.md") || strings.Contains(s, "opencode.http.json.example") {
			return true
		}
	}
	return false
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
