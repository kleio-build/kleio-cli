package commands

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

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
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			authURL := cfg.APIURL + "/auth/github"
			fmt.Printf("Opening browser for GitHub authentication...\n")
			fmt.Printf("If it doesn't open, visit: %s\n\n", authURL)

			openBrowser(authURL)

			fmt.Print("Paste the authorization code here: ")
			reader := bufio.NewReader(os.Stdin)
			code, _ := reader.ReadString('\n')
			code = strings.TrimSpace(code)

			if code == "" {
				return fmt.Errorf("no code provided")
			}

			c := client.New(cfg.APIURL, "", "")
			token, err := c.ExchangeCode(code)
			if err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}

			cfg.Token = token.AccessToken
			cfg.RefreshToken = token.RefreshToken

			tokenClient := client.NewWithToken(cfg.APIURL, cfg.Token, "")
			workspaces, err := tokenClient.ListWorkspaces()
			if err != nil {
				fmt.Printf("Warning: could not fetch workspaces: %v\n", err)
			} else if len(workspaces) == 1 {
				cfg.WorkspaceID = workspaces[0].ID
				fmt.Printf("Auto-selected workspace: %s\n", workspaces[0].Name)
			} else if len(workspaces) > 1 {
				fmt.Println("\nAvailable workspaces:")
				for i, ws := range workspaces {
					fmt.Printf("  [%d] %s (slug: %s)\n", i+1, ws.Name, ws.Slug)
				}
				fmt.Print("\nSelect workspace number: ")
				var selection int
				fmt.Scanln(&selection)
				if selection >= 1 && selection <= len(workspaces) {
					cfg.WorkspaceID = workspaces[selection-1].ID
					fmt.Printf("Selected workspace: %s\n", workspaces[selection-1].Name)
				}
			}

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Println("Authentication successful! Config saved to ~/.kleio/config.yaml")
			return nil
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
