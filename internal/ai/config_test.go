package ai

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProvider_Nil(t *testing.T) {
	p, err := ResolveProvider(nil)
	require.NoError(t, err)
	assert.IsType(t, Noop{}, p)
	assert.False(t, p.Available())
}

func TestResolveProvider_OpenAI(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "sk-test")
	p, err := ResolveProvider(&AIConfig{
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		APIKeyEnv: "TEST_OPENAI_KEY",
	})
	require.NoError(t, err)
	assert.IsType(t, &OpenAI{}, p)
	assert.True(t, p.Available())
}

func TestResolveProvider_OpenAI_MissingKey(t *testing.T) {
	t.Setenv("TEST_MISSING_KEY", "")
	p, err := ResolveProvider(&AIConfig{
		Provider:  "openai",
		APIKeyEnv: "TEST_MISSING_KEY",
	})
	require.NoError(t, err)
	assert.IsType(t, Noop{}, p)
}

func TestResolveProvider_Ollama(t *testing.T) {
	p, err := ResolveProvider(&AIConfig{
		Provider: "ollama",
		Model:    "llama3",
	})
	require.NoError(t, err)
	assert.IsType(t, &Ollama{}, p)
	assert.True(t, p.Available())
}

func TestResolveProvider_Unknown(t *testing.T) {
	_, err := ResolveProvider(&AIConfig{Provider: "unknown"})
	assert.Error(t, err)
}

func TestLoadConfigFrom_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
ai:
  provider: openai
  model: gpt-4o
  api_key_env: MY_KEY
`), 0o644))

	cfg := loadConfigFrom(path)
	require.NotNil(t, cfg)
	assert.Equal(t, "openai", cfg.Provider)
	assert.Equal(t, "gpt-4o", cfg.Model)
	assert.Equal(t, "MY_KEY", cfg.APIKeyEnv)
}

func TestLoadConfigFrom_NoAIBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`api_url: http://localhost`), 0o644))

	cfg := loadConfigFrom(path)
	assert.Nil(t, cfg)
}

func TestLoadConfigFrom_FileNotFound(t *testing.T) {
	cfg := loadConfigFrom("/nonexistent/path")
	assert.Nil(t, cfg)
}
