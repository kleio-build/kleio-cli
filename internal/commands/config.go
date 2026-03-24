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

	return cmd
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set a config value (api-url, api-key)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()

			switch args[0] {
			case "api-url":
				cfg.APIURL = args[1]
			case "api-key":
				cfg.APIKey = args[1]
			default:
				return fmt.Errorf("unknown config key: %s (valid: api-url, api-key)", args[0])
			}

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("Set %s = %s\n", args[0], args[1])
			return nil
		},
	}
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			fmt.Printf("api-url: %s\n", cfg.APIURL)
			fmt.Printf("api-key: %s\n", cfg.APIKey)
			fmt.Printf("config:  %s\n", config.DefaultPath())
			return nil
		},
	}
}
