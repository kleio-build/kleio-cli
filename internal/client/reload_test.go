package client

import (
	"testing"

	"github.com/kleio-build/kleio-cli/internal/config"
	"github.com/stretchr/testify/require"
)

func TestReloadFromConfig_UpdatesCredentials(t *testing.T) {
	c := NewWithTokens("http://localhost:8080", "old-token", "old-refresh", "ws-1")

	changed := c.ReloadFromConfig(&config.Config{
		APIURL:       "http://localhost:8080",
		Token:        "new-token",
		RefreshToken: "new-refresh",
		WorkspaceID:  "ws-2",
	})

	require.True(t, changed, "should report change")
	require.Equal(t, "ws-2", c.WorkspaceID())
}

func TestReloadFromConfig_NoChangeReturnsFalse(t *testing.T) {
	c := NewWithTokens("http://localhost:8080", "tok", "ref", "ws")

	changed := c.ReloadFromConfig(&config.Config{
		Token:        "tok",
		RefreshToken: "ref",
		APIKey:       "",
		WorkspaceID:  "ws",
	})

	require.False(t, changed, "should not report change when nothing differs")
}

func TestReloadFromConfig_NilConfigReturnsFalse(t *testing.T) {
	c := NewWithTokens("http://localhost:8080", "tok", "ref", "ws")
	require.False(t, c.ReloadFromConfig(nil))
}

func TestReloadFromConfig_EmptyWorkspaceKeepsExisting(t *testing.T) {
	c := NewWithTokens("http://localhost:8080", "tok", "ref", "ws-orig")

	changed := c.ReloadFromConfig(&config.Config{
		Token:        "tok",
		RefreshToken: "ref",
		WorkspaceID:  "",
	})

	require.False(t, changed)
	require.Equal(t, "ws-orig", c.WorkspaceID(), "workspace should not be overwritten by empty string")
}

func TestReloadFromConfig_TrimsWhitespace(t *testing.T) {
	c := NewWithTokens("http://localhost:8080", "old", "old-r", "ws")

	changed := c.ReloadFromConfig(&config.Config{
		Token:        "  new-token  ",
		RefreshToken: " new-ref ",
		WorkspaceID:  " ws-2 ",
	})

	require.True(t, changed)
	require.Equal(t, "ws-2", c.WorkspaceID())
}
