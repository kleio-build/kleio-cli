package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// demoWorkspaceID matches api/seed/001_demo_data.sql when using the default dev stack.
const demoWorkspaceID = "d0000001-0000-0000-0000-000000000001"

type Config struct {
	APIURL       string `yaml:"api_url"`
	APIKey       string `yaml:"api_key"`
	Token        string `yaml:"token,omitempty"`
	RefreshToken string `yaml:"refresh_token,omitempty"`
	WorkspaceID  string `yaml:"workspace_id,omitempty"`
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kleio", "config.yaml")
}

func Load() (*Config, error) {
	cfg := &Config{
		APIURL: "http://localhost:8080",
		APIKey: "kleio-dev-key",
	}

	if env := os.Getenv("KLEIO_API_URL"); env != "" {
		cfg.APIURL = env
	}
	if env := os.Getenv("KLEIO_API_KEY"); env != "" {
		cfg.APIKey = env
	}
	if env := os.Getenv("KLEIO_TOKEN"); env != "" {
		cfg.Token = env
	}
	if env := os.Getenv("KLEIO_WORKSPACE_ID"); env != "" {
		cfg.WorkspaceID = env
	}

	path := DefaultPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil
	}

	var fileCfg Config
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return cfg, nil
	}

	if fileCfg.APIURL != "" {
		cfg.APIURL = fileCfg.APIURL
	}
	if fileCfg.APIKey != "" {
		cfg.APIKey = fileCfg.APIKey
	}
	if fileCfg.Token != "" && cfg.Token == "" {
		cfg.Token = fileCfg.Token
	}
	if fileCfg.RefreshToken != "" && cfg.RefreshToken == "" {
		cfg.RefreshToken = fileCfg.RefreshToken
	}
	if fileCfg.WorkspaceID != "" && cfg.WorkspaceID == "" {
		cfg.WorkspaceID = fileCfg.WorkspaceID
	}

	// Local dev convenience: default workspace when using stock API URL + dev API key.
	if cfg.WorkspaceID == "" && cfg.APIKey == "kleio-dev-key" {
		u := strings.TrimSuffix(strings.TrimSpace(cfg.APIURL), "/")
		if u == "http://localhost:8080" || u == "http://127.0.0.1:8080" {
			cfg.WorkspaceID = demoWorkspaceID
		}
	}

	return cfg, nil
}

func Save(cfg *Config) error {
	path := DefaultPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}
