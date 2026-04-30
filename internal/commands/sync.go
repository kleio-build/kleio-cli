package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/kleio-build/kleio-cli/internal/storeutil"
	kleiosync "github.com/kleio-build/kleio-cli/internal/sync"
	"github.com/spf13/cobra"
)

// NewSyncCmd creates the `kleio sync` parent command with push/pull/auto subcommands.
func NewSyncCmd(getClient func() *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync local data with Kleio Cloud",
		Long:  "Upload local events/backlog to cloud, or download cloud data locally.",
	}

	cmd.AddCommand(newSyncPushCmd(getClient))
	cmd.AddCommand(newSyncPullCmd(getClient))
	cmd.AddCommand(newSyncAutoCmd(getClient))

	return cmd
}

func newSyncPushCmd(getClient func() *client.Client) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "push",
		Short: "Upload unsynced local data to the cloud",
		RunE: func(cmd *cobra.Command, args []string) error {
			localStore, err := resolveLocalStore()
			if err != nil {
				return fmt.Errorf("no local database found: %w", err)
			}

			apiClient := getClient()
			syncer := kleiosync.New(localStore, apiClient)

			result, err := syncer.Push(context.Background())
			if err != nil {
				return fmt.Errorf("push failed: %w", err)
			}

			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			fmt.Fprintf(os.Stdout, "Pushed: %d event(s), %d backlog item(s)",
				result.EventsPushed, result.BacklogPushed)
			if result.Errors > 0 {
				fmt.Fprintf(os.Stdout, " (%d error(s))", result.Errors)
			}
			fmt.Fprintln(os.Stdout)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func newSyncPullCmd(getClient func() *client.Client) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Download cloud data into local store",
		RunE: func(cmd *cobra.Command, args []string) error {
			localStore, err := resolveLocalStore()
			if err != nil {
				return fmt.Errorf("no local database found: %w", err)
			}

			apiClient := getClient()
			syncer := kleiosync.New(localStore, apiClient)

			result, err := syncer.Pull(context.Background())
			if err != nil {
				return fmt.Errorf("pull failed: %w", err)
			}

			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			fmt.Fprintf(os.Stdout, "Pulled: %d event(s), %d backlog item(s)\n",
				result.EventsPulled, result.BacklogPulled)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func newSyncAutoCmd(getClient func() *client.Client) *cobra.Command {
	var interval string

	cmd := &cobra.Command{
		Use:   "auto",
		Short: "Run background periodic sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			localStore, err := resolveLocalStore()
			if err != nil {
				return fmt.Errorf("no local database found: %w", err)
			}

			apiClient := getClient()
			syncer := kleiosync.New(localStore, apiClient)

			dur, err := time.ParseDuration(interval)
			if err != nil {
				return fmt.Errorf("invalid interval %q: %w", interval, err)
			}

			fmt.Fprintf(os.Stderr, "Starting auto-sync every %s (Ctrl-C to stop)\n", dur)
			ticker := time.NewTicker(dur)
			defer ticker.Stop()

			ctx := context.Background()

			syncOnce := func() {
				pushResult, pushErr := syncer.Push(ctx)
				if pushErr != nil {
					fmt.Fprintf(os.Stderr, "push error: %v\n", pushErr)
				} else {
					fmt.Fprintf(os.Stderr, "pushed: %d events, %d backlog\n",
						pushResult.EventsPushed, pushResult.BacklogPushed)
				}
				pullResult, pullErr := syncer.Pull(ctx)
				if pullErr != nil {
					fmt.Fprintf(os.Stderr, "pull error: %v\n", pullErr)
				} else {
					fmt.Fprintf(os.Stderr, "pulled: %d events, %d backlog\n",
						pullResult.EventsPulled, pullResult.BacklogPulled)
				}
			}

			syncOnce()
			for range ticker.C {
				syncOnce()
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&interval, "interval", "5m", "Sync interval (e.g. 5m, 1h)")
	return cmd
}

func resolveLocalStore() (*localdb.Store, error) {
	dbPath, err := storeutil.FindDBPath()
	if err != nil {
		return nil, err
	}
	db, err := localdb.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return localdb.New(db), nil
}
