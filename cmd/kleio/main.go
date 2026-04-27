package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/commands"
	"github.com/kleio-build/kleio-cli/internal/config"
	"github.com/kleio-build/kleio-cli/internal/gitowner"
	kleiomcp "github.com/kleio-build/kleio-cli/internal/mcp"
	"github.com/kleio-build/kleio-cli/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// newAuthenticatedClient builds an API client from a loaded disk config.
func newAuthenticatedClient(cfg *config.Config) *client.Client {
	var c *client.Client
	if cfg.Token != "" {
		c = client.NewWithTokens(cfg.APIURL, cfg.Token, cfg.RefreshToken, cfg.WorkspaceID)
		c.SetOnTokenRefresh(func(newToken, newRefreshToken string) {
			saved, _ := config.Load()
			saved.Token = newToken
			saved.RefreshToken = newRefreshToken
			_ = config.Save(saved)
		})
	} else {
		c = client.New(cfg.APIURL, cfg.APIKey, cfg.WorkspaceID)
	}
	if cfg.WorkspaceID == "" {
		resolveWorkspace(c)
	}
	return c
}

// startMCPConfigReload polls config files (pointer, environment file, legacy
// config.yaml) and reapplies credentials + base URL to the running MCP client
// so `kleio login` or `kleio config use` take effect without restarting Cursor.
func startMCPConfigReload(c *client.Client) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		var lastMaxMod int64
		maxMod := func() int64 {
			var m int64
			for _, p := range config.WatchPaths() {
				if st, err := os.Stat(p); err == nil {
					if t := st.ModTime().UnixNano(); t > m {
						m = t
					}
				}
			}
			return m
		}
		check := func() {
			mm := maxMod()
			if lastMaxMod != 0 && mm == lastMaxMod {
				return
			}
			lastMaxMod = mm
			next, err := config.Load()
			if err != nil {
				fmt.Fprintln(os.Stderr, "kleio mcp: config reload:", err)
				return
			}
			if c.ReloadFromConfig(next) {
				fmt.Fprintln(os.Stderr, "kleio mcp: reloaded config")
				if strings.TrimSpace(next.WorkspaceID) == "" {
					resolveWorkspace(c)
				}
			}
		}
		check()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				check()
			}
		}
	}()
	return cancel
}

func main() {
	getClient := func() *client.Client {
		cfg, _ := config.Load()
		return newAuthenticatedClient(cfg)
	}

	rootCmd := &cobra.Command{
		Use:     "kleio",
		Short:   "Capture work discovered during development",
		Long:    "Kleio turns work items discovered during AI-assisted development into a clean, context-rich backlog.",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version.Version, version.Commit, version.Date),
	}

	rootCmd.AddCommand(commands.NewCaptureCmd(getClient))
	rootCmd.AddCommand(commands.NewCheckpointCmd(getClient))
	rootCmd.AddCommand(commands.NewBacklogCmd(getClient))
	rootCmd.AddCommand(commands.NewQueryCmd(getClient))
	rootCmd.AddCommand(commands.NewDecideCmd(getClient))
	rootCmd.AddCommand(commands.NewConfigCmd())
	rootCmd.AddCommand(commands.NewLoginCmd(getClient))
	rootCmd.AddCommand(commands.NewLogoutCmd())
	rootCmd.AddCommand(commands.NewWorkspaceCmd(getClient))
	rootCmd.AddCommand(commands.NewInitCmd(getClient))
	rootCmd.AddCommand(commands.NewStatusCmd(getClient))
	rootCmd.AddCommand(commands.NewImportCmd(getClient))
	rootCmd.AddCommand(commands.NewCheckCmd(getClient))

	mcpCmd := &cobra.Command{
		Use:          "mcp",
		Short:        "Start the Kleio MCP server (stdio transport)",
		Long:         "Runs Kleio as a Model Context Protocol server over stdin/stdout for integration with AI editors like Cursor.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			apiClient := newAuthenticatedClient(cfg)
			stopReload := startMCPConfigReload(apiClient)
			defer stopReload()

			srv := kleiomcp.NewServer(apiClient)
			err = srv.Run(context.Background(), &mcp.StdioTransport{})
			if err != nil && (err == io.EOF || strings.Contains(err.Error(), "EOF")) {
				return nil
			}
			return err
		},
	}
	rootCmd.AddCommand(mcpCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// resolveWorkspace attempts to set the workspace on c when none was configured.
// Priority: git remote auto-detect > project-level .kleio/config.yaml.
func resolveWorkspace(c *client.Client) {
	wd, err := os.Getwd()
	if err != nil {
		return
	}

	// 1. Git remote: match GitHub owner against user's workspaces.
	if owner := gitowner.DetectOwner(wd); owner != "" {
		workspaces, err := c.ListWorkspaces()
		if err == nil {
			for _, ws := range workspaces {
				if ws.GitHubOwnerLogin != nil && strings.EqualFold(*ws.GitHubOwnerLogin, owner) {
					c.SetWorkspaceID(ws.ID)
					return
				}
			}
		}
	}

	// 2. Project config: .kleio/config.yaml in project root or ancestor.
	if projCfg := config.LoadProject(wd); projCfg != nil && projCfg.WorkspaceID != "" {
		c.SetWorkspaceID(projCfg.WorkspaceID)
	}
}
