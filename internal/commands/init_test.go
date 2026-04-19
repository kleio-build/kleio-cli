package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kleio-build/kleio-cli/internal/config"
)

// withIsolatedHome forces ~/.kleio resolution into a temp dir so config tests
// don't mutate the user's real config.
func withIsolatedHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("KLEIO_ENV", "")
	t.Setenv("KLEIO_API_URL", "")
	t.Setenv("KLEIO_API_KEY", "")
	t.Setenv("KLEIO_TOKEN", "")
	t.Setenv("KLEIO_WORKSPACE_ID", "")
	return dir
}

func TestResolveSurfaceTool(t *testing.T) {
	cases := []struct {
		name        string
		tool        string
		surface     string
		want        string
		expectError bool
	}{
		{"both empty", "", "", "", false},
		{"tool only", "cursor", "", "cursor", false},
		{"surface only", "", "cursor", "cursor", false},
		{"github surface normalises to copilot", "", "github", "copilot", false},
		{"matching tool+surface ok", "cursor", "cursor", "cursor", false},
		{"matching case-insensitive ok", "cursor", "CURSOR", "cursor", false},
		{"conflict rejected", "cursor", "claude", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveSurfaceTool(tc.tool, tc.surface)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil (got=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

// SC-INIT1 — missing --tool/--surface in non-interactive mode is a clear error.
func TestValidateNonInteractiveFlags_MissingTool(t *testing.T) {
	withIsolatedHome(t)
	err := validateNonInteractiveFlags("", "https://api.example/", "ws-1")
	if err == nil || !strings.Contains(err.Error(), "--surface") {
		t.Fatalf("expected --surface error, got %v", err)
	}
}

// SC-INIT1 — missing --api-url with no preset is a clear error.
func TestValidateNonInteractiveFlags_MissingAPIURL(t *testing.T) {
	dir := withIsolatedHome(t)
	envFile := filepath.Join(dir, ".kleio", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(envFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte("api_url: \"\"\nworkspace_id: \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("KLEIO_API_URL", "")

	err := validateNonInteractiveFlagsWithEmptyDefault(t, "cursor", "", "")
	if err == nil {
		t.Fatal("expected api-url error, got nil")
	}
}

// validateNonInteractiveFlagsWithEmptyDefault is a thin wrapper that asserts
// the production-default api_url ("https://api.kleio.build") doesn't mask
// missing flags. We force-clear it via a temp env file.
func validateNonInteractiveFlagsWithEmptyDefault(t *testing.T, tool, apiURL, workspaceID string) error {
	t.Helper()
	cfg, _ := config.Load()
	cfg.APIURL = ""
	cfg.WorkspaceID = ""
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return validateNonInteractiveFlags(tool, apiURL, workspaceID)
}

// SC-INIT1 — missing --workspace-id with no preset is a clear error.
func TestValidateNonInteractiveFlags_MissingWorkspace(t *testing.T) {
	withIsolatedHome(t)
	cfg, _ := config.Load()
	cfg.APIURL = "https://api.example/"
	cfg.WorkspaceID = ""
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	err := validateNonInteractiveFlags("cursor", "", "")
	if err == nil || !strings.Contains(err.Error(), "--workspace-id") {
		t.Fatalf("expected --workspace-id error, got %v", err)
	}
}

// SC-INIT1 — happy path: all required fields satisfied via flags.
func TestValidateNonInteractiveFlags_Happy(t *testing.T) {
	withIsolatedHome(t)
	cfg, _ := config.Load()
	cfg.APIURL = ""
	cfg.WorkspaceID = ""
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := validateNonInteractiveFlags("cursor", "https://api.example/", "ws-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// applyNonInteractiveOverrides should write both fields into the active
// environment config so subsequent reads see them.
func TestApplyNonInteractiveOverrides_PersistsBothFields(t *testing.T) {
	withIsolatedHome(t)
	if err := applyNonInteractiveOverrides("https://api.example/v1", "ws-42"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != "https://api.example/v1" {
		t.Fatalf("api_url not persisted, got %q", cfg.APIURL)
	}
	if cfg.WorkspaceID != "ws-42" {
		t.Fatalf("workspace_id not persisted, got %q", cfg.WorkspaceID)
	}
}

// applyNonInteractiveOverrides with both fields blank is a no-op.
func TestApplyNonInteractiveOverrides_NoopWhenEmpty(t *testing.T) {
	withIsolatedHome(t)
	if err := applyNonInteractiveOverrides("", ""); err != nil {
		t.Fatalf("expected no-op, got %v", err)
	}
}

// SC-OC3 — `kleio init --surface opencode --non-interactive` installs the
// opencode bootstrap files into the target dir. This is a true integration
// test: it touches the real filesystem (in t.TempDir()) and the real
// embedded templates.
func TestRunInit_SurfaceOpenCodeWritesBootstrapFiles(t *testing.T) {
	withIsolatedHome(t)
	cfg, _ := config.Load()
	cfg.APIURL = "https://api.example/"
	cfg.WorkspaceID = "00000000-0000-0000-0000-000000000001"
	cfg.APIKey = "kleio-test-key"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()
	err := runInit(InitFlags{
		Dir:            projectDir,
		NonInteractive: true,
		Surface:        "opencode",
	}, nil)
	if err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	// SC-OC1/SC-OC3: opencode.json.example lands at the project root.
	if _, statErr := os.Stat(filepath.Join(projectDir, "opencode.json.example")); statErr != nil {
		t.Fatalf("opencode.json.example missing at project root: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(projectDir, "opencode.http.json.example")); statErr != nil {
		t.Fatalf("opencode.http.json.example missing at project root: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(projectDir, "AGENTS.opencode.md")); statErr != nil {
		t.Fatalf("AGENTS.opencode.md missing at project root: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(projectDir, ".opencode", "hooks", "kleio-auth-check.sh")); statErr != nil {
		t.Fatalf(".opencode/hooks/kleio-auth-check.sh missing: %v", statErr)
	}
	// SC-INIT2: surface=opencode does NOT install other surfaces.
	if _, statErr := os.Stat(filepath.Join(projectDir, ".cursor")); statErr == nil {
		t.Fatal("--surface opencode should not install .cursor/")
	}
	if _, statErr := os.Stat(filepath.Join(projectDir, ".claude")); statErr == nil {
		t.Fatal("--surface opencode should not install .claude/")
	}
}

// Conflicting --interactive + --non-interactive in the entry point.
func TestRunInit_RejectsBothModes(t *testing.T) {
	withIsolatedHome(t)
	err := runInit(InitFlags{
		Dir:            ".",
		Interactive:    true,
		NonInteractive: true,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "either --interactive or --non-interactive") {
		t.Fatalf("expected mutual-exclusion error, got %v", err)
	}
}
