package ai

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AIConfig represents the `ai:` block in ~/.kleio/config.yaml.
type AIConfig struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	APIKeyEnv string `yaml:"api_key_env"`
	BaseURL   string `yaml:"base_url,omitempty"`
}

type fileRoot struct {
	AI *AIConfig `yaml:"ai"`
}

// LoadConfig reads the AI configuration from ~/.kleio/config.yaml.
// Returns nil when the file is absent or the `ai:` block is empty.
func LoadConfig() *AIConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return loadConfigFrom(filepath.Join(home, ".kleio", "config.yaml"))
}

func loadConfigFrom(path string) *AIConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var root fileRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil
	}
	if root.AI == nil || root.AI.Provider == "" {
		return nil
	}
	return root.AI
}

// ResolveProvider returns the appropriate Provider based on the config.
// Returns Noop when config is nil.
func ResolveProvider(cfg *AIConfig) (Provider, error) {
	if cfg == nil {
		return Noop{}, nil
	}

	apiKey := ""
	if cfg.APIKeyEnv != "" {
		apiKey = os.Getenv(cfg.APIKeyEnv)
		if apiKey == "" {
			return Noop{}, nil
		}
	}

	switch cfg.Provider {
	case "openai":
		model := cfg.Model
		if model == "" {
			model = "gpt-4o-mini"
		}
		return NewOpenAI(apiKey, model, cfg.BaseURL), nil
	case "anthropic":
		model := cfg.Model
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}
		return NewAnthropic(apiKey, model), nil
	case "gemini":
		model := cfg.Model
		if model == "" {
			model = "gemini-2.0-flash"
		}
		return NewGemini(apiKey, model), nil
	case "ollama":
		base := cfg.BaseURL
		if base == "" {
			base = "http://localhost:11434"
		}
		model := cfg.Model
		if model == "" {
			model = "llama3"
		}
		return NewOllama(base, model), nil
	default:
		return nil, fmt.Errorf("unknown ai provider %q", cfg.Provider)
	}
}
