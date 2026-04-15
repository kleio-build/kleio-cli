package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func homeEnv(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func clearKleioEnv(t *testing.T) {
	t.Helper()
	t.Setenv("KLEIO_ENV", "")
	t.Setenv("KLEIO_API_URL", "")
	t.Setenv("KLEIO_API_KEY", "")
	t.Setenv("KLEIO_TOKEN", "")
	t.Setenv("KLEIO_WORKSPACE_ID", "")
}

// --- Load: defaults ---

func TestLoad_DefaultsToProductionURL(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, DefaultProductionAPIURL, cfg.APIURL)
	require.Empty(t, cfg.APIKey)
	require.Empty(t, cfg.Environment)
}

// --- Load: pointer file ---

func TestLoad_PointerToStaging(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	writePointer(t, home, "staging")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, EnvStaging, cfg.Environment)
	require.Equal(t, "https://api.dev.kleio.build", cfg.APIURL)
}

func TestLoad_PointerToLocal_DemoWorkspace(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	writePointer(t, home, "local")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, EnvLocal, cfg.Environment)
	require.Equal(t, "http://localhost:8080", cfg.APIURL)
	require.Equal(t, devAPIKey, cfg.APIKey)
	require.Equal(t, demoWorkspaceID, cfg.WorkspaceID)
}

func TestLoad_InvalidPointer(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	writePointer(t, home, "bogus")

	_, err := Load()
	require.Error(t, err)
}

// --- Load: environment config file ---

func TestLoad_EnvironmentFileOverridesPresetURL(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	writePointer(t, home, "staging")
	writeEnvFile(t, home, "staging", "api_url: https://custom.staging.example.com\n")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, EnvStaging, cfg.Environment)
	require.Equal(t, "https://custom.staging.example.com", cfg.APIURL)
}

func TestLoad_EnvironmentFilePreservesTokens(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	writePointer(t, home, "production")
	writeEnvFile(t, home, "production",
		"token: my-prod-token\nrefresh_token: my-refresh\nworkspace_id: ws-prod\n")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "my-prod-token", cfg.Token)
	require.Equal(t, "my-refresh", cfg.RefreshToken)
	require.Equal(t, "ws-prod", cfg.WorkspaceID)
}

func TestLoad_EnvironmentFileInvalidYAML(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	writePointer(t, home, "staging")
	writeEnvFile(t, home, "staging", "not: valid: yaml: :\n")

	_, err := Load()
	require.Error(t, err)
}

// --- Load: KLEIO_ENV overrides ---

func TestLoad_KleioEnvOverridesPointer(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)
	t.Setenv("KLEIO_ENV", "staging")

	writePointer(t, home, "local")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, EnvStaging, cfg.Environment)
	require.Equal(t, "https://api.dev.kleio.build", cfg.APIURL)
}

func TestLoad_KleioEnvInvalid(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)
	t.Setenv("KLEIO_ENV", "bogus")

	_, err := Load()
	require.Error(t, err)
}

// --- Load: env var overrides ---

func TestLoad_KleioAPIURLOverridesAll(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)
	t.Setenv("KLEIO_ENV", "staging")
	t.Setenv("KLEIO_API_URL", "http://override:9090")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, EnvStaging, cfg.Environment)
	require.Equal(t, "http://override:9090", cfg.APIURL)
}

func TestLoad_EnvVarsOverrideFileCredentials(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)
	t.Setenv("KLEIO_API_KEY", "env-key")
	t.Setenv("KLEIO_TOKEN", "env-tok")

	writePointer(t, home, "production")
	writeEnvFile(t, home, "production", "api_key: file-key\ntoken: file-tok\n")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "env-key", cfg.APIKey)
	require.Equal(t, "env-tok", cfg.Token)
}

// --- Load: legacy fallback ---

func TestLoad_LegacyConfigFallback(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	kleioDir := filepath.Join(home, ".kleio")
	require.NoError(t, os.MkdirAll(kleioDir, 0o755))
	content := "api_url: http://localhost:8080\napi_key: legacy-key\ntoken: legacy-tok\n"
	require.NoError(t, os.WriteFile(filepath.Join(kleioDir, "config.yaml"), []byte(content), 0o600))

	cfg, err := Load()
	require.NoError(t, err)
	require.Empty(t, cfg.Environment)
	require.Equal(t, "http://localhost:8080", cfg.APIURL)
	require.Equal(t, "legacy-key", cfg.APIKey)
	require.Equal(t, "legacy-tok", cfg.Token)
}

func TestLoad_LegacyConfigInvalidYAML(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	kleioDir := filepath.Join(home, ".kleio")
	require.NoError(t, os.MkdirAll(kleioDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(kleioDir, "config.yaml"), []byte("not: valid: yaml: :\n"), 0o600))

	_, err := Load()
	require.Error(t, err)
}

// --- Load: local dev convenience ---

func TestLoad_LocalDevConvenience_OnlyInLocal(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	// Legacy config pointing to localhost with dev key but no environment pointer:
	// demo workspace should NOT be set because Environment is empty.
	kleioDir := filepath.Join(home, ".kleio")
	require.NoError(t, os.MkdirAll(kleioDir, 0o755))
	content := "api_url: http://localhost:8080\napi_key: " + devAPIKey + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(kleioDir, "config.yaml"), []byte(content), 0o600))

	cfg, err := Load()
	require.NoError(t, err)
	require.Empty(t, cfg.Environment)
	require.Empty(t, cfg.WorkspaceID, "demo workspace only triggers for explicit local environment")
}

// --- Save ---

func TestSave_WritesToEnvironmentFile(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)

	cfg := &Config{
		Environment: EnvStaging,
		APIURL:      "https://api.dev.kleio.build",
		Token:       "stg-tok",
		WorkspaceID: "ws-stg",
	}
	require.NoError(t, Save(cfg))

	data, err := os.ReadFile(EnvironmentConfigPath(EnvStaging))
	require.NoError(t, err)
	require.Contains(t, string(data), "stg-tok")
	require.NotContains(t, string(data), "environment:")
}

func TestSave_WritesToLegacyWhenNoEnvironment(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)

	cfg := &Config{APIURL: "http://custom:1234", Token: "custom-tok"}
	require.NoError(t, Save(cfg))

	data, err := os.ReadFile(DefaultPath())
	require.NoError(t, err)
	require.Contains(t, string(data), "custom-tok")
}

// --- SetActiveEnvironment / ActiveEnvironment ---

func TestSetActiveEnvironment_Valid(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	require.NoError(t, SetActiveEnvironment("staging"))
	require.Equal(t, EnvStaging, ActiveEnvironment())
}

func TestSetActiveEnvironment_NormalizesAlias(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	require.NoError(t, SetActiveEnvironment("prod"))
	require.Equal(t, EnvProduction, ActiveEnvironment())
}

func TestSetActiveEnvironment_Invalid(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	require.Error(t, SetActiveEnvironment("bogus"))
}

func TestActiveEnvironment_NoFile(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
	clearKleioEnv(t)

	require.Empty(t, ActiveEnvironment())
}

// --- EnsureEnvironmentFile ---

func TestEnsureEnvironmentFile_Creates(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)

	require.NoError(t, EnsureEnvironmentFile("staging"))

	data, err := os.ReadFile(EnvironmentConfigPath(EnvStaging))
	require.NoError(t, err)
	require.Contains(t, string(data), "https://api.dev.kleio.build")
}

func TestEnsureEnvironmentFile_ExistingNotOverwritten(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)

	path := EnvironmentConfigPath(EnvStaging)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("token: keep-me\n"), 0o600))

	require.NoError(t, EnsureEnvironmentFile("staging"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "keep-me")
}

// --- PresetForEnv ---

func TestPresetForEnv_Aliases(t *testing.T) {
	n, u, k, err := PresetForEnv("prod")
	require.NoError(t, err)
	require.Equal(t, EnvProduction, n)
	require.Equal(t, DefaultProductionAPIURL, u)
	require.Empty(t, k)
}

func TestPresetForEnv_LocalHasDevKey(t *testing.T) {
	n, u, k, err := PresetForEnv("dev")
	require.NoError(t, err)
	require.Equal(t, EnvLocal, n)
	require.Equal(t, "http://localhost:8080", u)
	require.Equal(t, devAPIKey, k)
}

// --- LoadProject (unchanged) ---

func TestLoadProject_ReturnsWorkspaceWhenFileExists(t *testing.T) {
	root := t.TempDir()
	kleioDir := filepath.Join(root, ".kleio")
	require.NoError(t, os.MkdirAll(kleioDir, 0o755))
	cfgPath := filepath.Join(kleioDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("workspace_id: ws-from-project\n"), 0o600))

	got := LoadProject(root)
	require.NotNil(t, got)
	require.Equal(t, "ws-from-project", got.WorkspaceID)
}

// --- helpers ---

func writePointer(t *testing.T, home, env string) {
	t.Helper()
	dir := filepath.Join(home, ".kleio")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "environment"), []byte(env+"\n"), 0o600))
}

func writeEnvFile(t *testing.T, home, env, content string) {
	t.Helper()
	dir := filepath.Join(home, ".kleio", "environments")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, env+".yaml"), []byte(content), 0o600))
}
