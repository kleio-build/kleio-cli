package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/cursorimport"
	"github.com/kleio-build/kleio-cli/internal/privacy"
	"github.com/spf13/cobra"
)

func newImportCursorCmd(getStore func() kleio.Store) *cobra.Command {
	var (
		project string
		dryRun  bool
	)

	cmd := &cobra.Command{
		Use:   "cursor",
		Short: "Import Cursor agent transcripts as Kleio captures",
		Long: `Discovers and parses Cursor agent-transcript JSONL files, extracting
decisions, work items, and checkpoints from tool-call sequences.

Signals that were already captured via Kleio MCP tools are identified
and skipped. The privacy filter redacts credentials before submission.

Examples:
  kleio import cursor --dry-run
  kleio import cursor --project c-Users-brenn-my-project`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var transcripts []string
			var err error

			if project != "" {
				transcripts, err = cursorimport.DiscoverTranscriptsForProject(project)
			} else {
				transcripts, err = cursorimport.DiscoverTranscripts()
			}
			if err != nil {
				return fmt.Errorf("discover transcripts: %w", err)
			}

			if len(transcripts) == 0 {
				fmt.Println("No Cursor agent transcripts found.")
				return nil
			}

			fmt.Printf("Found %d transcript files. Parsing...\n", len(transcripts))

			parser := cursorimport.NewTranscriptParser()
			pf := privacy.NewFilter(privacy.DefaultRules())

			var allSignals []cursorimport.Signal
			var totalFiles int
			seenHashes := make(map[string]bool)

			for _, path := range transcripts {
				result, err := parser.ParseFile(path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", filepath.Base(path), err)
					continue
				}
				totalFiles++

				for _, sig := range result.Signals {
					hash := sig.Hash()
					if seenHashes[hash] {
						continue
					}
					seenHashes[hash] = true
					allSignals = append(allSignals, sig)
				}
			}

			newSignals := filterNewSignals(allSignals)
			alreadyCaptured := len(allSignals) - len(newSignals)

			fmt.Printf("Parsed %d files: %d signals found (%d already captured, %d new)\n",
				totalFiles, len(allSignals), alreadyCaptured, len(newSignals))

			if len(newSignals) == 0 {
				fmt.Println("Nothing new to import.")
				return nil
			}

			if dryRun {
				fmt.Println("\nDry run — would import:")
				for _, sig := range newSignals {
					content := truncate(pf.Redact(sig.Content), 80)
					fmt.Printf("  [%s] %s\n", sig.SignalType, content)
				}
				return nil
			}

			store := getStore()
			ctx := context.Background()
			imported := 0
			for _, sig := range newSignals {
				content := pf.Redact(sig.Content)
				evt := buildCursorEvent(sig, content)
				if err := store.CreateEvent(ctx, evt); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to import signal: %v\n", err)
					continue
				}
				imported++
			}

			fmt.Printf("\nImported %d/%d new signals as captures.\n", imported, len(newSignals))
			return nil
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Cursor project slug to import from (default: all projects)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be imported without sending")
	return cmd
}

func filterNewSignals(signals []cursorimport.Signal) []cursorimport.Signal {
	var result []cursorimport.Signal
	for _, s := range signals {
		if !s.AlreadyCaptured {
			result = append(result, s)
		}
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func buildCursorEvent(sig cursorimport.Signal, redactedContent string) *kleio.Event {
	sd := map[string]interface{}{
		"ingest_source": "cursor_transcript",
		"signal_hash":   sig.Hash(),
	}
	if sig.SourceFile != "" {
		sd["file"] = sig.SourceFile
	}
	sdJSON, _ := json.Marshal(sd)

	provenance := "Imported from Cursor agent transcript"
	if sig.SourceFile != "" {
		provenance += " (" + filepath.Base(sig.SourceFile) + ")"
	}

	return &kleio.Event{
		Content:         redactedContent,
		SignalType:      sig.SignalType,
		SourceType:      kleio.SourceTypeCursorTranscript,
		StructuredData:  string(sdJSON),
		FreeformContext: provenance,
	}
}
