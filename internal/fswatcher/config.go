package fswatcher

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type WatchConfig struct {
	Enabled bool     `yaml:"enabled"`
	Sources []string `yaml:"sources"`
}

// LoadWatchConfig reads watch configuration from ~/.kleio/config.yaml.
// Returns a disabled config if the file doesn't exist or has no watch section.
func LoadWatchConfig() WatchConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return WatchConfig{}
	}

	data, err := os.ReadFile(filepath.Join(home, ".kleio", "config.yaml"))
	if err != nil {
		return WatchConfig{}
	}

	var wrapper struct {
		Watch WatchConfig `yaml:"watch"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return WatchConfig{}
	}

	return wrapper.Watch
}

func (c WatchConfig) HasSource(name string) bool {
	for _, s := range c.Sources {
		if s == name {
			return true
		}
	}
	return false
}
