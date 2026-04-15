package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kleio-build/kleio-cli/internal/config"
	"github.com/stretchr/testify/require"
)

func testHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("KLEIO_ENV", "")
	t.Setenv("KLEIO_API_URL", "")
	t.Setenv("KLEIO_API_KEY", "")
	t.Setenv("KLEIO_TOKEN", "")
	t.Setenv("KLEIO_WORKSPACE_ID", "")
}

func TestConfigUse_ValidEnv(t *testing.T) {
	home := t.TempDir()
	testHome(t, home)

	cmd := newConfigUseCmd()
	cmd.SetArgs([]string{"staging"})
	require.NoError(t, cmd.Execute())

	require.Equal(t, config.EnvStaging, config.ActiveEnvironment())

	path := config.EnvironmentConfigPath(config.EnvStaging)
	_, err := os.Stat(path)
	require.NoError(t, err)
}

func TestConfigUse_InvalidEnv(t *testing.T) {
	home := t.TempDir()
	testHome(t, home)

	cmd := newConfigUseCmd()
	cmd.SetArgs([]string{"not-a-real-env"})
	require.Error(t, cmd.Execute())
}

func TestConfigUse_PreservesOtherEnvTokens(t *testing.T) {
	home := t.TempDir()
	testHome(t, home)

	// Set up staging with tokens
	require.NoError(t, config.SetActiveEnvironment("staging"))
	require.NoError(t, config.EnsureEnvironmentFile("staging"))
	stgCfg, err := config.Load()
	require.NoError(t, err)
	stgCfg.Token = "staging-token"
	stgCfg.RefreshToken = "staging-refresh"
	stgCfg.WorkspaceID = "ws-stg"
	require.NoError(t, config.Save(stgCfg))

	// Switch to production
	cmd := newConfigUseCmd()
	cmd.SetArgs([]string{"production"})
	require.NoError(t, cmd.Execute())

	// Staging tokens are still in the staging file
	stgPath := config.EnvironmentConfigPath(config.EnvStaging)
	data, err := os.ReadFile(stgPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "staging-token")
	require.Contains(t, string(data), "staging-refresh")
	require.Contains(t, string(data), "ws-stg")

	// Active env is now production
	require.Equal(t, config.EnvProduction, config.ActiveEnvironment())
}

func TestConfigSetAPIURL_ModifiesActiveConfig(t *testing.T) {
	home := t.TempDir()
	testHome(t, home)

	require.NoError(t, config.SetActiveEnvironment("staging"))
	require.NoError(t, config.EnsureEnvironmentFile("staging"))

	cmd := newConfigSetCmd()
	cmd.SetArgs([]string{"api-url", "https://custom.example.com"})
	require.NoError(t, cmd.Execute())

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, "https://custom.example.com", cfg.APIURL)
	require.Equal(t, config.EnvStaging, cfg.Environment)

	// Verify it's in the staging environment file
	data, err := os.ReadFile(config.EnvironmentConfigPath(config.EnvStaging))
	require.NoError(t, err)
	require.Contains(t, string(data), "https://custom.example.com")
}

func TestConfigUse_SwitchBackRestoresTokens(t *testing.T) {
	home := t.TempDir()
	testHome(t, home)

	// Set up staging with a token
	require.NoError(t, config.SetActiveEnvironment("staging"))
	require.NoError(t, config.EnsureEnvironmentFile("staging"))
	stgCfg, err := config.Load()
	require.NoError(t, err)
	stgCfg.Token = "staging-token"
	require.NoError(t, config.Save(stgCfg))

	// Set up production with a different token
	require.NoError(t, config.SetActiveEnvironment("production"))
	require.NoError(t, config.EnsureEnvironmentFile("production"))
	prodCfg, err := config.Load()
	require.NoError(t, err)
	prodCfg.Token = "prod-token"
	require.NoError(t, config.Save(prodCfg))

	// Switch back to staging
	useCmd := newConfigUseCmd()
	useCmd.SetArgs([]string{"staging"})
	require.NoError(t, useCmd.Execute())

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, config.EnvStaging, cfg.Environment)
	require.Equal(t, "staging-token", cfg.Token)

	// Production token still intact
	envDir := filepath.Join(home, ".kleio", "environments")
	data, err := os.ReadFile(filepath.Join(envDir, "production.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "prod-token")
}
