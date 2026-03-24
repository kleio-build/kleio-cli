package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/config"
	"github.com/kleio-build/kleio-cli/internal/mcpdetect"
	"github.com/spf13/cobra"
)

func NewStatusCmd(getClient func() *client.Client) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show CLI configuration, API connectivity, and workspace summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := config.DefaultPath()
			cfg, err := config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Config: invalid (%v)\n", err)
				fmt.Println("Status: not ready")
				os.Exit(1)
			}

			apiURL := strings.TrimSpace(cfg.APIURL)
			if apiURL == "" {
				fmt.Println("Config: invalid (api_url is empty)")
				fmt.Printf("Config file: %s\n", cfgPath)
				fmt.Println("Status: not ready")
				os.Exit(1)
			}
			fmt.Println("Config: ok")
			if _, err := os.Stat(cfgPath); err == nil {
				fmt.Printf("Config file: %s (exists)\n", cfgPath)
			} else {
				fmt.Printf("Config file: %s (not found; using defaults/env)\n", cfgPath)
			}

			wd, _ := os.Getwd()
			mcp := mcpdetect.Scan(wd)
			if mcp.Configured {
				fmt.Println("MCP: configured (kleio + mcp found in .cursor/mcp.json)")
				if mcp.ProjectHasKleio {
					fmt.Printf("  project: %s\n", mcp.ProjectConfigPath)
				}
				if mcp.UserHasKleio {
					fmt.Printf("  user:    %s\n", mcp.UserConfigPath)
				}
			} else {
				fmt.Println("MCP: not configured (no kleio+mcp entry in project or user .cursor/mcp.json)")
			}
			fmt.Printf("MCP: process — %s\n", mcp.ProcessHint)

			if err := client.HealthCheck(apiURL, nil); err != nil {
				fmt.Printf("API: unreachable (%v)\n", err)
				fmt.Println("Status: not ready")
				os.Exit(1)
			}
			fmt.Println("API: reachable (GET /api/health ok)")

			hasCreds := strings.TrimSpace(cfg.APIKey) != "" || strings.TrimSpace(cfg.Token) != ""
			if !hasCreds {
				fmt.Println("Auth: skipped (set api_key or run kleio login)")
				fmt.Println("Status: not ready (missing credentials)")
				os.Exit(1)
			}

			cl := getClient()
			me, err := cl.GetMeJSON()
			if err != nil {
				fmt.Printf("Auth: failed (%v)\n", err)
				fmt.Println("Status: not ready")
				os.Exit(1)
			}
			if am, ok := me["auth_mode"].(string); ok && am != "" {
				fmt.Printf("Auth: ok (mode: %s)\n", am)
			} else {
				fmt.Println("Auth: ok")
			}

			ws := strings.TrimSpace(cfg.WorkspaceID)
			if ws == "" {
				fmt.Println("Workspace: not set (configure workspace_id or kleio workspace)")
				fmt.Println("Status: not ready")
				os.Exit(1)
			}

			if asJSON {
				raw, err := cl.GetWorkspaceCountsRaw()
				if err != nil {
					fmt.Fprintf(os.Stderr, "workspace counts: %v\n", err)
					os.Exit(1)
				}
				fmt.Print(string(raw))
				if len(raw) > 0 && raw[len(raw)-1] != '\n' {
					fmt.Println()
				}
				return nil
			}

			counts, err := cl.GetWorkspaceCounts()
			if err != nil {
				fmt.Printf("Workspace counts: failed (%v)\n", err)
				fmt.Println("Status: not ready")
				os.Exit(1)
			}

			fmt.Println()
			fmt.Println("Workspace summary")
			fmt.Printf("  id:     %s\n", counts.WorkspaceID)
			if counts.WorkspaceName != "" {
				fmt.Printf("  name:   %s\n", counts.WorkspaceName)
			}
			if counts.WorkspaceSlug != "" {
				fmt.Printf("  slug:   %s\n", counts.WorkspaceSlug)
			}
			fmt.Printf("  captures: %d\n", counts.CapturesCount)
			fmt.Printf("  backlog:  %d\n", counts.BacklogItemsCount)
			if len(counts.BacklogByStatus) > 0 {
				fmt.Println("  backlog by status:")
				for k, v := range counts.BacklogByStatus {
					fmt.Printf("    %s: %d\n", k, v)
				}
			}

			fmt.Println()
			fmt.Println("Status: ready for use")
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print raw JSON for workspace counts response")
	return cmd
}
