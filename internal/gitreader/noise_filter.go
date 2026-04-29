package gitreader

import (
	"path/filepath"
	"strings"
)

type NoiseConfig struct {
	Enabled       bool
	LockfileNames map[string]bool
}

func DefaultNoiseConfig() NoiseConfig {
	return NoiseConfig{
		Enabled: true,
		LockfileNames: map[string]bool{
			"package-lock.json": true,
			"yarn.lock":         true,
			"pnpm-lock.yaml":    true,
			"go.sum":            true,
			"Gemfile.lock":      true,
			"Cargo.lock":        true,
			"poetry.lock":       true,
			"composer.lock":     true,
			"Pipfile.lock":      true,
		},
	}
}

type NoiseFilter struct {
	cfg NoiseConfig
}

func NewNoiseFilter(cfg NoiseConfig) *NoiseFilter {
	return &NoiseFilter{cfg: cfg}
}

func (f *NoiseFilter) Apply(commits []Commit) []Commit {
	if !f.cfg.Enabled {
		return commits
	}
	var result []Commit
	for _, c := range commits {
		if f.isNoise(c) {
			continue
		}
		result = append(result, c)
	}
	return result
}

func (f *NoiseFilter) isNoise(c Commit) bool {
	if c.IsMerge {
		return true
	}
	if strings.TrimSpace(c.Message) == "" {
		return true
	}
	if len(c.Files) > 0 && f.allLockfiles(c.Files) {
		return true
	}
	return false
}

func (f *NoiseFilter) allLockfiles(files []string) bool {
	for _, file := range files {
		base := filepath.Base(file)
		if !f.cfg.LockfileNames[base] {
			return false
		}
	}
	return true
}
