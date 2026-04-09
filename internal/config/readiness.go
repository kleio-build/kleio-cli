package config

import (
	"strings"
)

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
