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

func TestLoad_ReturnsErrorWhenConfigFileYAMLIsInvalid(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)

	kleioDir := filepath.Join(home, ".kleio")
	require.NoError(t, os.MkdirAll(kleioDir, 0o755))
	cfgPath := filepath.Join(kleioDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("not: valid: yaml: :\n"), 0o600))

	_, err := Load()
	require.Error(t, err)
}

func TestLoad_MergesAPIURLFromValidConfigFile(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)

	kleioDir := filepath.Join(home, ".kleio")
	require.NoError(t, os.MkdirAll(kleioDir, 0o755))
	cfgPath := filepath.Join(kleioDir, "config.yaml")
	content := "api_url: https://api.example.test\napi_key: key-from-file\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o600))

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "https://api.example.test", cfg.APIURL)
	require.Equal(t, "key-from-file", cfg.APIKey)
}

func TestLoad_WhenConfigFileMissing_ReturnsDefaultsWithoutError(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotEmpty(t, cfg.APIURL)
}

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
