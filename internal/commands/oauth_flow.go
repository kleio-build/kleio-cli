package commands

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/config"
)

// RunOAuthLoginFlow authenticates via GitHub's Device Authorization Flow
// (RFC 8628). The user is shown a short code to enter on github.com/login/device.
func RunOAuthLoginFlow(r *bufio.Reader) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	c := client.New(cfg.APIURL, "", "")
	deviceCode, err := c.RequestDeviceCode()
	if err != nil {
		return fmt.Errorf("failed to request device code: %w", err)
	}

	fmt.Println("Opening browser to authorize...")
	fmt.Println()
	fmt.Printf("  Your code: %s\n", deviceCode.UserCode)
	fmt.Printf("  Visit:     %s\n", deviceCode.VerificationURI)
	fmt.Println()

	openBrowser(deviceCode.VerificationURI)

	interval := deviceCode.Interval
	if interval < 5 {
		interval = 5
	}

	fmt.Print("Waiting for authorization...")
	for {
		time.Sleep(time.Duration(interval) * time.Second)
		fmt.Print(".")

		result, err := c.PollDeviceToken(deviceCode.DeviceCode)
		if err != nil {
			fmt.Println()
			return fmt.Errorf("polling failed: %w", err)
		}

		if result.Pending {
			if result.Interval > 0 {
				interval = result.Interval
			}
			continue
		}

		if result.Error != "" {
			fmt.Println()
			return fmt.Errorf("authorization failed: %s", result.Error)
		}

		fmt.Println(" done!")

		cfg.Token = result.AccessToken
		cfg.RefreshToken = result.RefreshToken

		tokenClient := client.NewWithToken(cfg.APIURL, cfg.Token, "")
		if err := pickWorkspaceInteractive(cfg, tokenClient, r); err != nil {
			return err
		}

		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Println("Authentication successful! Config saved to ~/.kleio/config.yaml")
		fmt.Println()
		fmt.Println("Tip: If the Kleio MCP server is running, restart it to use your new credentials.")
		return nil
	}
}

// PickWorkspaceIfNeeded lists workspaces and sets cfg.WorkspaceID when missing but token is set.
func PickWorkspaceIfNeeded(cfg *config.Config, api *client.Client, r *bufio.Reader) error {
	if strings.TrimSpace(cfg.WorkspaceID) != "" {
		return nil
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil
	}
	return pickWorkspaceInteractive(cfg, api, r)
}

func pickWorkspaceInteractive(cfg *config.Config, tokenClient *client.Client, r *bufio.Reader) error {
	workspaces, err := tokenClient.ListWorkspaces()
	if err != nil {
		fmt.Printf("Warning: could not fetch workspaces: %v\n", err)
		return nil
	}
	if len(workspaces) == 0 {
		return nil
	}
	if len(workspaces) == 1 {
		cfg.WorkspaceID = workspaces[0].ID
		fmt.Printf("Auto-selected workspace: %s\n", workspaces[0].Name)
		return nil
	}

	fmt.Println("\nAvailable workspaces:")
	for i, ws := range workspaces {
		fmt.Printf("  [%d] %s (slug: %s)\n", i+1, ws.Name, ws.Slug)
	}
	fmt.Print("\nSelect workspace number (or enter slug): ")
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	if n, err := strconv.Atoi(line); err == nil {
		if n >= 1 && n <= len(workspaces) {
			cfg.WorkspaceID = workspaces[n-1].ID
			fmt.Printf("Selected workspace: %s\n", workspaces[n-1].Name)
			return nil
		}
	}
	for _, ws := range workspaces {
		if ws.Slug == line || ws.ID == line {
			cfg.WorkspaceID = ws.ID
			fmt.Printf("Selected workspace: %s\n", ws.Name)
			return nil
		}
	}
	fmt.Printf("Workspace '%s' not found; leaving workspace unset.\n", line)
	return nil
}

// RunAPIKeySetup prompts for api-url (optional) and api-key and saves config.
func RunAPIKeySetup(r *bufio.Reader) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fmt.Printf("API URL [%s]: ", cfg.APIURL)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	line = strings.TrimSpace(line)
	if line != "" {
		cfg.APIURL = line
	}
	fmt.Print("API key: ")
	key, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("api key required")
	}
	cfg.APIKey = key
	cfg.Token = ""
	cfg.RefreshToken = ""
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Println("Saved API key to ~/.kleio/config.yaml")
	return nil
}

// StdinReader returns a bufio.Reader for stdin (shared pattern).
func StdinReader() *bufio.Reader {
	return bufio.NewReader(os.Stdin)
}
