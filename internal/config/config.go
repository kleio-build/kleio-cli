package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const demoWorkspaceID = "d0000001-0000-0000-0000-000000000001"

const DefaultProductionAPIURL = "https://api.kleio.build"

const devAPIKey = "kleio-dev-key"

const (
	EnvProduction = "production"
	EnvStaging    = "staging"
	EnvLocal      = "local"
)

type Config struct {
	// Environment is derived from the pointer file or KLEIO_ENV; never serialized to YAML.
	Environment  string `yaml:"-"`
	APIURL       string `yaml:"api_url"`
	APIKey       string `yaml:"api_key"`
	Token        string `yaml:"token,omitempty"`
	RefreshToken string `yaml:"refresh_token,omitempty"`
	WorkspaceID  string `yaml:"workspace_id,omitempty"`
}

// PresetForEnv resolves a user-facing name to its canonical form, API base URL,
// and default API key (non-empty only for local).
func PresetForEnv(name string) (normalized, apiURL, defaultAPIKey string, err error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "production", "prod":
		return EnvProduction, DefaultProductionAPIURL, "", nil
	case "staging", "stage":
		return EnvStaging, "https://api.dev.kleio.build", "", nil
	case "local", "dev":
		return EnvLocal, "http://localhost:8080", devAPIKey, nil
	default:
		return "", "", "", fmt.Errorf("unknown Kleio environment %q (use production, staging, or local)", name)
	}
}

func kleioDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kleio")
}

// DefaultPath returns ~/.kleio/config.yaml (legacy single-file config).
func DefaultPath() string {
	return filepath.Join(kleioDir(), "config.yaml")
}

func environmentPointerPath() string {
	return filepath.Join(kleioDir(), "environment")
}

// EnvironmentConfigPath returns ~/.kleio/environments/<env>.yaml.
func EnvironmentConfigPath(env string) string {
	return filepath.Join(kleioDir(), "environments", env+".yaml")
}

// ActiveEnvironment reads the pointer from ~/.kleio/environment.
// Returns "" when the file is absent (legacy or fresh install).
func ActiveEnvironment() string {
	data, err := os.ReadFile(environmentPointerPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// SetActiveEnvironment validates name and writes the pointer file.
func SetActiveEnvironment(name string) error {
	norm, _, _, err := PresetForEnv(name)
	if err != nil {
		return err
	}
	dir := kleioDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(environmentPointerPath(), []byte(norm+"\n"), 0o600)
}

// EnsureEnvironmentFile creates ~/.kleio/environments/<env>.yaml with preset
// defaults if the file does not already exist.
func EnsureEnvironmentFile(env string) error {
	norm, presetURL, defKey, err := PresetForEnv(env)
	if err != nil {
		return err
	}
	path := EnvironmentConfigPath(norm)
	if _, statErr := os.Stat(path); statErr == nil {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	seed := &Config{APIURL: presetURL, APIKey: defKey}
	data, err := yaml.Marshal(seed)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// WatchPaths returns all file paths that Load() may read, for change-detection
// by the MCP config-reload loop.
func WatchPaths() []string {
	paths := []string{environmentPointerPath(), DefaultPath()}
	if env := ActiveEnvironment(); env != "" {
		paths = append(paths, EnvironmentConfigPath(env))
	}
	return paths
}

// Load resolves the active configuration using the following priority:
//
//  1. Active environment: KLEIO_ENV env var > ~/.kleio/environment pointer > (none)
//  2. Environment file: ~/.kleio/environments/<env>.yaml (or legacy ~/.kleio/config.yaml when no env)
//  3. Env-var overrides: KLEIO_API_URL, KLEIO_API_KEY, KLEIO_TOKEN, KLEIO_WORKSPACE_ID
//  4. Local dev convenience: demo workspace when env is "local" with default API key
func Load() (*Config, error) {
	cfg := &Config{
		APIURL: DefaultProductionAPIURL,
	}

	kleioEnv := strings.TrimSpace(os.Getenv("KLEIO_ENV"))
	activeEnv := kleioEnv
	if activeEnv == "" {
		activeEnv = ActiveEnvironment()
	}

	if activeEnv != "" {
		norm, presetURL, defKey, err := PresetForEnv(activeEnv)
		if err != nil {
			return nil, err
		}
		cfg.Environment = norm
		cfg.APIURL = presetURL
		if norm == EnvLocal {
			cfg.APIKey = defKey
		}

		envPath := EnvironmentConfigPath(norm)
		if data, readErr := os.ReadFile(envPath); readErr == nil {
			var envCfg Config
			if parseErr := yaml.Unmarshal(data, &envCfg); parseErr != nil {
				return nil, fmt.Errorf("parse %s: %w", envPath, parseErr)
			}
			mergeFile(cfg, &envCfg)
		}
	} else {
		legacyPath := DefaultPath()
		if data, readErr := os.ReadFile(legacyPath); readErr == nil {
			var fileCfg Config
			if parseErr := yaml.Unmarshal(data, &fileCfg); parseErr != nil {
				return nil, fmt.Errorf("parse %s: %w", legacyPath, parseErr)
			}
			mergeFile(cfg, &fileCfg)
		}
	}

	if v := os.Getenv("KLEIO_API_URL"); v != "" {
		cfg.APIURL = v
	}
	if v := os.Getenv("KLEIO_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("KLEIO_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("KLEIO_WORKSPACE_ID"); v != "" {
		cfg.WorkspaceID = v
	}

	if cfg.Environment == EnvLocal && cfg.WorkspaceID == "" && cfg.APIKey == devAPIKey {
		cfg.WorkspaceID = demoWorkspaceID
	}

	return cfg, nil
}

func mergeFile(dst, src *Config) {
	if src.APIURL != "" {
		dst.APIURL = src.APIURL
	}
	if src.APIKey != "" {
		dst.APIKey = src.APIKey
	}
	if src.Token != "" {
		dst.Token = src.Token
	}
	if src.RefreshToken != "" {
		dst.RefreshToken = src.RefreshToken
	}
	if src.WorkspaceID != "" {
		dst.WorkspaceID = src.WorkspaceID
	}
}

// Save writes the config to the active environment file
// (~/.kleio/environments/<env>.yaml) or to legacy config.yaml when
// Environment is empty.
func Save(cfg *Config) error {
	var path string
	if cfg.Environment != "" {
		path = EnvironmentConfigPath(cfg.Environment)
	} else {
		path = DefaultPath()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0o600)
}

// LoadProject walks up from startDir looking for a .kleio/config.yaml file.
// Returns the parsed config if found (only workspace_id is expected), or nil if
// no project-level config exists.
func LoadProject(startDir string) *Config {
	dir := startDir
	for {
		candidate := filepath.Join(dir, ".kleio", "config.yaml")
		data, err := os.ReadFile(candidate)
		if err == nil {
			var cfg Config
			if yaml.Unmarshal(data, &cfg) == nil && cfg.WorkspaceID != "" {
				return &cfg
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil
}
