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

// InitFlags captures the resolved CLI flags for `kleio init`. Grouping them
// keeps the runInit signature stable as we add programmatic-bootstrap fields
// (used by kleio-eval to wire ephemeral workspaces non-interactively).
type InitFlags struct {
	Dir            string
	DryRun         bool
	YesNewOnly     bool
	Interactive    bool
	NonInteractive bool
	ForceOverwrite bool
	Tool           string
	Surface        string
	APIURL         string
	WorkspaceID    string
}

func NewInitCmd(getClient func() *client.Client) *cobra.Command {
	f := InitFlags{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Install Kleio agent templates (AGENTS.md, editor rules, examples)",
		Long: `Installs Kleio templates for your editor workflow.

Use --interactive (-i) for the full wizard (tooling, optional sign-in, workspace).
Use --non-interactive with --tool/--surface for CI or programmatic bootstrap
(no prompts; writes sidecar files when paths exist).

Programmatic bootstrap (used by kleio-eval and CI):
  kleio init --non-interactive --surface cursor \
             --api-url https://api.dev.kleio.build \
             --workspace-id 0a1b2c3d-... \
             --dir ./worktree`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(f, getClient)
		},
	}
	cmd.Flags().StringVar(&f.Dir, "dir", ".", "project root to write files into")
	cmd.Flags().BoolVar(&f.DryRun, "dry-run", false, "print actions without writing files")
	cmd.Flags().BoolVar(&f.YesNewOnly, "yes-new-only", false, "do not overwrite existing files; write kleio sidecar files instead")
	cmd.Flags().BoolVarP(&f.Interactive, "interactive", "i", false, "run interactive wizard (tooling, auth, workspace)")
	cmd.Flags().BoolVar(&f.NonInteractive, "non-interactive", false, "no prompts; requires --tool/--surface when the profile cannot be inferred")
	cmd.Flags().BoolVar(&f.ForceOverwrite, "force-overwrite", false, "overwrite existing files without prompting")
	cmd.Flags().StringVar(&f.Tool, "tool", "", "tool profile: cursor, claude, windsurf, copilot, codex, opencode, generic, none, all, or comma-separated (e.g. cursor,claude)")
	cmd.Flags().StringVar(&f.Surface, "surface", "", "alias of --tool used by kleio-eval (one of: cursor, claude, codex, windsurf, github, copilot, opencode, generic)")
	cmd.Flags().StringVar(&f.APIURL, "api-url", "", "non-interactive: write this api_url to the active environment config")
	cmd.Flags().StringVar(&f.WorkspaceID, "workspace-id", "", "non-interactive: write this workspace_id to the active environment config")
	return cmd
}

// resolveSurfaceTool merges the legacy --tool flag and the new --surface alias.
// If both are provided they must agree. The "github" surface is normalised to
// "copilot" so existing initprofile.ID values still resolve.
func resolveSurfaceTool(tool, surface string) (string, error) {
	t := strings.TrimSpace(tool)
	s := strings.TrimSpace(strings.ToLower(surface))
	if s == "github" {
		s = "copilot"
	}
	if t != "" && s != "" && !strings.EqualFold(t, s) {
		return "", fmt.Errorf("conflicting --tool=%q and --surface=%q (use one)", t, s)
	}
	if t != "" {
		return t, nil
	}
	return s, nil
}

// applyNonInteractiveOverrides writes --api-url and --workspace-id into the
// active environment config so the rest of the flow (verify, MCP) sees them.
// SC-INIT1 requires both to be present in true non-interactive mode if neither
// is already configured; we don't enforce it here so callers can opt in to
// partial overrides, but runInit gates required-field validation.
func applyNonInteractiveOverrides(apiURL, workspaceID string) error {
	if apiURL == "" && workspaceID == "" {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if apiURL != "" {
		cfg.APIURL = apiURL
	}
	if workspaceID != "" {
		cfg.WorkspaceID = workspaceID
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func runInit(f InitFlags, getClient func() *client.Client) error {
	if f.Interactive && f.NonInteractive {
		return fmt.Errorf("use either --interactive or --non-interactive, not both")
	}
	tool, err := resolveSurfaceTool(f.Tool, f.Surface)
	if err != nil {
		return err
	}
	if f.NonInteractive {
		if err := applyNonInteractiveOverrides(f.APIURL, f.WorkspaceID); err != nil {
			return err
		}
		if err := validateNonInteractiveFlags(tool, f.APIURL, f.WorkspaceID); err != nil {
			return err
		}
	}
	return runInitInner(f.Dir, f.DryRun, f.YesNewOnly, f.Interactive, f.NonInteractive, f.ForceOverwrite, tool, getClient)
}

// validateNonInteractiveFlags enforces SC-INIT1: a clear, non-zero error when
// the operator forgot a required field. We only fail when nothing in the
// resolved config covers the missing piece (env-var or pre-existing config can
// substitute for the explicit flag, which lets CI prefer env vars when
// preferred).
func validateNonInteractiveFlags(tool, apiURL, workspaceID string) error {
	if strings.TrimSpace(tool) == "" {
		return fmt.Errorf("--non-interactive requires --tool or --surface (e.g. --surface cursor)")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.APIURL) == "" && strings.TrimSpace(apiURL) == "" {
		return fmt.Errorf("--non-interactive requires --api-url (or KLEIO_API_URL / preset config)")
	}
	if strings.TrimSpace(cfg.WorkspaceID) == "" && strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("--non-interactive requires --workspace-id (or KLEIO_WORKSPACE_ID / preset config)")
	}
	return nil
}

func runInitInner(dir string, dryRun, yesNewOnly, interactive, nonInteractive, forceOverwrite bool, tool string, getClient func() *client.Client) error {
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
			return nil, fmt.Errorf("could not infer tool profile (no .cursor / .claude / .windsurf / .codex / .opencode / marker files); pass --tool=cursor|claude|windsurf|copilot|codex|opencode|generic|none|all")
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
			strings.HasPrefix(s, ".opencode/") ||
			s == "CLAUDE.md" || s == ".github/copilot-instructions.md" ||
			s == "opencode.json" {
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
	fmt.Print("Which editor/tooling should Kleio install for? (cursor|claude|windsurf|copilot|codex|opencode|generic|none|all) [", rec, "]: ")
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
