package config

import (
	"os"
	"strings"
)

// ConfigFileExists reports whether the default config file is present on disk.
func ConfigFileExists() bool {
	_, err := os.Stat(DefaultPath())
	return err == nil
}

// HasAuth returns true when either API key or OAuth token is present (after env merge).
func HasAuth(c *Config) bool {
	if c == nil {
		return false
	}
	return strings.TrimSpace(c.APIKey) != "" || strings.TrimSpace(c.Token) != ""
}

// HasWorkspace returns true when a workspace id is set.
func HasWorkspace(c *Config) bool {
	if c == nil {
		return false
	}
	return strings.TrimSpace(c.WorkspaceID) != ""
}

// UsingOAuth is true when an OAuth token is configured.
func UsingOAuth(c *Config) bool {
	if c == nil {
		return false
	}
	return strings.TrimSpace(c.Token) != ""
}

// UsingAPIKey is true when an API key is configured (including dev defaults).
func UsingAPIKey(c *Config) bool {
	if c == nil {
		return false
	}
	return strings.TrimSpace(c.APIKey) != ""
}
