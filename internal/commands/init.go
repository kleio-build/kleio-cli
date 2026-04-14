package commands

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kleio-build/kleio-cli/internal/bootstrap"
	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/config"
	"github.com/kleio-build/kleio-cli/internal/initprofile"
	"github.com/spf13/cobra"
)

func NewInitCmd(getClient func() *client.Client) *cobra.Command {
	var (
		dir            string
		dryRun         bool
		yesNewOnly     bool
		interactive    bool
		nonInteractive bool
		forceOverwrite bool
		tool           string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Install Kleio agent templates (AGENTS.md, editor rules, examples)",
		Long: `Installs Kleio templates for your editor workflow.

Use --interactive (-i) for the full wizard (tooling, optional sign-in, workspace).
Use --non-interactive with --tool for CI (no prompts; writes sidecar files when paths exist).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if interactive && nonInteractive {
				return fmt.Errorf("use either --interactive or --non-interactive, not both")
			}
			return runInit(dir, dryRun, yesNewOnly, interactive, nonInteractive, forceOverwrite, tool, getClient)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "project root to write files into")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print actions without writing files")
	cmd.Flags().BoolVar(&yesNewOnly, "yes-new-only", false, "do not overwrite existing files; write kleio sidecar files instead")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "run interactive wizard (tooling, auth, workspace)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "no prompts; requires --tool when the profile cannot be inferred")
	cmd.Flags().BoolVar(&forceOverwrite, "force-overwrite", false, "overwrite existing files without prompting")
	cmd.Flags().StringVar(&tool, "tool", "", "tool profile: cursor, claude, windsurf, copilot, codex, generic, none, all, or comma-separated (e.g. cursor,claude)")
	return cmd
}

func runInit(dir string, dryRun, yesNewOnly, interactive, nonInteractive, forceOverwrite bool, tool string, getClient func() *client.Client) error {
	dir = filepath.Clean(dir)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("invalid --dir: %s", dir)
	}

	fsys, err := bootstrap.TemplateFS()
	if err != nil {
		return err
	}

	tty := isTTYStdin()
	r := bufio.NewReader(os.Stdin)
	autoSidecar := nonInteractive || yesNewOnly
	canPrompt := interactive && tty && !autoSidecar && !forceOverwrite && !dryRun

	ids, err := resolveProfiles(dir, interactive, nonInteractive, tool, r)
	if err != nil {
		return err
	}

	var rels []string
	if len(ids) == 1 && ids[0] == initprofile.None {
		rels = nil
	} else {
		rels, err = initprofile.MergeProfiles(ids)
		if err != nil {
			return err
		}
	}

	mode := "Kleio init"
	if interactive {
		mode += " (interactive)"
	}
	fmt.Println(mode + " — installing templates…")

	if len(rels) > 0 {
		fmt.Printf("Profiles: %s\n", formatProfileIDs(ids))
		sig := initprofile.DetectSignals(dir)
		if len(sig) > 0 {
			fmt.Println("Detected project signals:", strings.Join(sig, ", "))
		} else {
			fmt.Println("Detected project signals: none (greenfield).")
		}
		fmt.Println()
		fmt.Println("Planned install:")
		for _, embedRel := range rels {
			fmt.Printf("  → %s\n", initprofile.EmbedToDestRel(embedRel))
		}
	} else {
		fmt.Println("Profile `none` selected — skipping template files.")
	}

	if dryRun {
		fmt.Println()
		fmt.Println("(dry-run: no files will be written)")
	}

	var written []string
	var skipped int

	for _, embedRel := range rels {
		data, err := fs.ReadFile(fsys, embedRel)
		if err != nil {
			return fmt.Errorf("read template %s: %w", embedRel, err)
		}

		destRel := initprofile.EmbedToDestRel(embedRel)
		canonical := filepath.Join(dir, filepath.FromSlash(destRel))
		dest := canonical
		useSidecar := false
		allowOverwrite := forceOverwrite

		if _, statErr := os.Stat(canonical); statErr == nil {
			if forceOverwrite {
				allowOverwrite = true
			} else if canPrompt {
				fmt.Printf("\nConflict: %s already exists.\nOverwrite? [y/N]: ", canonical)
				line, _ := r.ReadString('\n')
				line = strings.TrimSpace(strings.ToLower(line))
				if line == "y" || line == "yes" {
					allowOverwrite = true
				} else {
					useSidecar = true
				}
			} else {
				useSidecar = true
			}
		}

		if useSidecar {
			sc := initprofile.SidecarPath(destRel)
			dest = filepath.Join(dir, filepath.FromSlash(sc))
			fmt.Printf("Writing sidecar instead: %s\n", filepath.ToSlash(dest))
		}

		if _, statErr := os.Stat(dest); statErr == nil && !allowOverwrite {
			fmt.Printf("skip (exists): %s\n", dest)
			skipped++
			continue
		}

		if dryRun {
			fmt.Printf("would write: %s (%d bytes)\n", dest, len(data))
			written = append(written, dest)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if strings.HasSuffix(strings.ToLower(dest), ".sh") {
			mode = 0755
		}
		if err := os.WriteFile(dest, data, mode); err != nil {
			return err
		}
		fmt.Printf("wrote: %s\n", dest)
		written = append(written, dest)
	}

	fmt.Printf("\nDone. %d file(s) written, %d skipped.\n", len(written), skipped)

	// Interactive auth + workspace (embedded login flow)
	if interactive && !nonInteractive {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !config.HasAuth(cfg) {
			fmt.Println()
			fmt.Println("Sign in to Kleio (same session — browser will open).")
			fmt.Println("  [1] GitHub sign-in (recommended)")
			fmt.Println("  [2] Use an API key instead")
			fmt.Print("Choose 1 or 2 [1]: ")
			line, err := r.ReadString('\n')
			if err != nil {
				return err
			}
			line = strings.TrimSpace(line)
			if line == "" || line == "1" {
				if err := RunOAuthLoginFlow(r); err != nil {
					return err
				}
			} else if line == "2" {
				if err := RunAPIKeySetup(r); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("invalid choice (use 1 or 2)")
			}
		}
		cfg, err = config.Load()
		if err != nil {
			return err
		}
		if config.HasAuth(cfg) && !config.HasWorkspace(cfg) {
			c := clientForInit(cfg)
			if err := PickWorkspaceIfNeeded(cfg, c, r); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	printReadiness(cfg)

	verifyOK := false
	verifyDetail := ""
	if config.HasAuth(cfg) && config.HasWorkspace(cfg) {
		if err := RunInitVerify(getClient); err != nil {
			verifyDetail = err.Error()
			printInitVerify(false, verifyDetail)
		} else {
			verifyOK = true
			printInitVerify(true, "")
		}
	} else {
		fmt.Println()
		fmt.Println("Init verify skipped (complete auth and workspace to verify API access).")
	}

	printNextSteps(ids, written, dir, verifyOK)

	return nil
}

func profileIDsInclude(ids []initprofile.ID, id initprofile.ID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func isTTYStdin() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func clientForInit(cfg *config.Config) *client.Client {
	if strings.TrimSpace(cfg.Token) != "" {
		return client.NewWithToken(cfg.APIURL, cfg.Token, cfg.WorkspaceID)
	}
	return client.New(cfg.APIURL, cfg.APIKey, cfg.WorkspaceID)
}

func resolveProfiles(dir string, interactive, nonInteractive bool, tool string, r *bufio.Reader) ([]initprofile.ID, error) {
	if tool != "" {
		return initprofile.ParseList(tool)
	}
	if nonInteractive {
		if !hasProfileSignal(dir) {
			return nil, fmt.Errorf("could not infer tool profile (no .cursor / .claude / .windsurf / .codex / marker files); pass --tool=cursor|claude|windsurf|copilot|codex|generic|none|all")
		}
		return []initprofile.ID{initprofile.Recommend(dir)}, nil
	}
	if interactive {
		return promptToolProfile(dir, r)
	}
	return []initprofile.ID{initprofile.Recommend(dir)}, nil
}

func hasProfileSignal(dir string) bool {
	for _, s := range initprofile.DetectSignals(dir) {
		if strings.HasPrefix(s, ".cursor/") || strings.HasPrefix(s, ".claude/") ||
			strings.HasPrefix(s, ".windsurf/") || strings.HasPrefix(s, ".codex/") ||
			s == "CLAUDE.md" || s == ".github/copilot-instructions.md" {
			return true
		}
	}
	return false
}

func promptToolProfile(dir string, r *bufio.Reader) ([]initprofile.ID, error) {
	rec := initprofile.Recommend(dir)
	sig := initprofile.DetectSignals(dir)
	if len(sig) > 0 {
		fmt.Printf("Recommended tool profile: %s\n", rec)
	} else {
		fmt.Println("We could not infer your editor from the repo.")
		fmt.Printf("Recommended tool profile: %s\n", rec)
	}
	fmt.Print("Which editor/tooling should Kleio install for? (cursor|claude|windsurf|copilot|codex|generic|none|all) [", rec, "]: ")
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return []initprofile.ID{rec}, nil
	}
	return initprofile.ParseList(line)
}

func formatProfileIDs(ids []initprofile.ID) string {
	var s []string
	for _, id := range ids {
		s = append(s, string(id))
	}
	return strings.Join(s, ", ")
}
