package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/commands"
	"github.com/kleio-build/kleio-cli/internal/config"
	"github.com/kleio-build/kleio-cli/internal/gitowner"
	kleiomcp "github.com/kleio-build/kleio-cli/internal/mcp"
	"github.com/kleio-build/kleio-cli/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func main() {
	getClient := func() *client.Client {
		cfg, _ := config.Load()
		var c *client.Client
		if cfg.Token != "" {
			c = client.NewWithTokens(cfg.APIURL, cfg.Token, cfg.RefreshToken, cfg.WorkspaceID)
			c.SetOnTokenRefresh(func(newToken, newRefreshToken string) {
				cfg.Token = newToken
				cfg.RefreshToken = newRefreshToken
				_ = config.Save(cfg)
			})
		} else {
			c = client.New(cfg.APIURL, cfg.APIKey, cfg.WorkspaceID)
		}
		if cfg.WorkspaceID == "" {
			resolveWorkspace(c)
		}
		return c
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
	rootCmd.AddCommand(commands.NewDecideCmd(getClient))
	rootCmd.AddCommand(commands.NewObserveCmd(getClient))
	rootCmd.AddCommand(commands.NewConfigCmd())
	rootCmd.AddCommand(commands.NewLoginCmd(getClient))
	rootCmd.AddCommand(commands.NewLogoutCmd())
	rootCmd.AddCommand(commands.NewWorkspaceCmd(getClient))
	rootCmd.AddCommand(commands.NewInitCmd(getClient))
	rootCmd.AddCommand(commands.NewStatusCmd(getClient))

	mcpCmd := &cobra.Command{
		Use:          "mcp",
		Short:        "Start the Kleio MCP server (stdio transport)",
		Long:         "Runs Kleio as a Model Context Protocol server over stdin/stdout for integration with AI editors like Cursor.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := kleiomcp.NewServer(getClient())
			err := srv.Run(context.Background(), &mcp.StdioTransport{})
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
