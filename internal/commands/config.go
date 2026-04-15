package commands

import (
	"fmt"

	"github.com/kleio-build/kleio-cli/internal/config"
	"github.com/spf13/cobra"
)

func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration",
	}

	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigUseCmd())

	return cmd
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set a config value (api-url, api-key, workspace-id)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			switch args[0] {
			case "api-url":
				cfg.APIURL = args[1]
			case "api-key":
				cfg.APIKey = args[1]
			case "workspace-id":
				cfg.WorkspaceID = args[1]
			default:
				return fmt.Errorf("unknown config key: %s (valid: api-url, api-key, workspace-id)", args[0])
			}

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			if args[0] == "api-key" {
				fmt.Printf("Set %s = <redacted>\n", args[0])
			} else {
				fmt.Printf("Set %s = %s\n", args[0], args[1])
			}
			return nil
		},
	}
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			envDisplay := cfg.Environment
			if envDisplay == "" {
				envDisplay = "(none — using legacy config or defaults)"
			}
			fmt.Printf("environment:   %s\n", envDisplay)
			fmt.Printf("api-url:       %s\n", cfg.APIURL)
			fmt.Printf("api-key:       %s\n", redactSecret(cfg.APIKey))
			fmt.Printf("token:         %s\n", redactSecret(cfg.Token))
			fmt.Printf("workspace-id:  %s\n", cfg.WorkspaceID)

			path := config.DefaultPath()
			if cfg.Environment != "" {
				path = config.EnvironmentConfigPath(cfg.Environment)
			}
			fmt.Printf("config file:   %s\n", path)
			return nil
		},
	}
}

func newConfigUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use [environment]",
		Short: "Switch API environment (production, staging, local)",
		Long: "Switches the active Kleio environment. Each environment stores its own\n" +
			"credentials, so you only need to run 'kleio login' once per environment.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.SetActiveEnvironment(args[0]); err != nil {
				return err
			}
			if err := config.EnsureEnvironmentFile(args[0]); err != nil {
				return err
			}

			norm, url, _, _ := config.PresetForEnv(args[0])
			cfg, _ := config.Load()
			if cfg != nil && cfg.Token != "" {
				fmt.Printf("Switched to %s (%s)\n", norm, url)
			} else {
				fmt.Printf("Switched to %s (%s)\n", norm, url)
				fmt.Println("Run 'kleio login' to authenticate.")
			}
			return nil
		},
	}
}

func redactSecret(s string) string {
	if s == "" {
		return "(empty)"
	}
	return "(set, hidden)"
}
