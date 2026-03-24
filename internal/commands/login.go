package commands

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/config"
	"github.com/spf13/cobra"
)

func NewLoginCmd(getClient func() *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Kleio via GitHub OAuth",
		Long:  "Opens your browser for GitHub authentication, then stores the access token locally.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunOAuthLoginFlow(bufio.NewReader(os.Stdin))
		},
	}

	return cmd
}

func NewLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear stored authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cfg.Token = ""
			cfg.RefreshToken = ""
			cfg.WorkspaceID = ""
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Println("Logged out. Cleared stored token.")
			return nil
		},
	}
}

func NewWorkspaceCmd(getClient func() *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage workspace context",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List available workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			c := client.NewWithToken(cfg.APIURL, cfg.Token, "")
			workspaces, err := c.ListWorkspaces()
			if err != nil {
				return fmt.Errorf("failed to list workspaces: %w", err)
			}
			for _, ws := range workspaces {
				active := " "
				if ws.ID == cfg.WorkspaceID {
					active = "*"
				}
				fmt.Printf(" %s %s (slug: %s, plan: %s)\n", active, ws.Name, ws.Slug, ws.Plan)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "select [slug-or-id]",
		Short: "Select active workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			c := client.NewWithToken(cfg.APIURL, cfg.Token, "")
			workspaces, err := c.ListWorkspaces()
			if err != nil {
				return fmt.Errorf("failed to list workspaces: %w", err)
			}
			target := args[0]
			for _, ws := range workspaces {
				if ws.ID == target || ws.Slug == target {
					cfg.WorkspaceID = ws.ID
					if err := config.Save(cfg); err != nil {
						return err
					}
					fmt.Printf("Active workspace: %s\n", ws.Name)
					return nil
				}
			}
			return fmt.Errorf("workspace '%s' not found", target)
		},
	})

	return cmd
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}
