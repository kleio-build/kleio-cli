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
	kleiomcp "github.com/kleio-build/kleio-cli/internal/mcp"
	"github.com/kleio-build/kleio-cli/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func main() {
	var cfg *config.Config

	getClient := func() *client.Client {
		if cfg == nil {
			cfg, _ = config.Load()
		}
		if cfg.Token != "" {
			return client.NewWithToken(cfg.APIURL, cfg.Token, cfg.WorkspaceID)
		}
		return client.New(cfg.APIURL, cfg.APIKey, cfg.WorkspaceID)
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
