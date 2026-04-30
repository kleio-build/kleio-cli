package storeutil

import (
	"os"
	"path/filepath"
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_CloudMode_WhenAuthAndWorkspace(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		APIURL:      "https://api.kleio.build",
		APIKey:      "test-key",
		WorkspaceID: "ws-123",
	}

	// Act
	s, err := Resolve(cfg, func() *client.Client {
		return client.New("https://api.kleio.build", "test-key", "ws-123")
	})
	require.NoError(t, err)
	defer s.Close()

	// Assert
	assert.Equal(t, kleio.StoreModeCloud, s.Mode())
}

func TestResolve_LocalMode_WhenNoAuth(t *testing.T) {
	// Arrange
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(tmp))
	defer os.Chdir(orig)

	cfg := &config.Config{}

	// Act
	s, err := Resolve(cfg, nil)
	require.NoError(t, err)
	defer s.Close()

	// Assert
	assert.Equal(t, kleio.StoreModeLocal, s.Mode())
	assert.FileExists(t, filepath.Join(tmp, KleioDir, DBFile))
}

func TestFindDBPath_FindsExistingDB(t *testing.T) {
	// Arrange
	tmp := t.TempDir()
	subdir := filepath.Join(tmp, "a", "b")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	dbDir := filepath.Join(tmp, KleioDir)
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dbDir, DBFile), nil, 0o600))

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(subdir))
	defer os.Chdir(orig)

	// Act
	path, err := FindDBPath()
	require.NoError(t, err)

	// Assert
	assert.Equal(t, filepath.Join(tmp, KleioDir, DBFile), path)
}

func TestFindDBPath_DefaultsToCwd(t *testing.T) {
	// Arrange
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(tmp))
	defer os.Chdir(orig)

	// Act
	path, err := FindDBPath()
	require.NoError(t, err)

	// Assert
	assert.Equal(t, filepath.Join(tmp, KleioDir, DBFile), path)
}
