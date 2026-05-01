package ai

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

var (
	autoOnce     sync.Once
	autoProvider Provider
)

// AutoDetect returns a Provider from explicit config if present;
// otherwise pings localhost Ollama with a 300ms timeout and returns it
// if reachable. The result is cached per process via sync.Once.
func AutoDetect(cfg *AIConfig) Provider {
	autoOnce.Do(func() {
		autoProvider = doAutoDetect(cfg)
	})
	return autoProvider
}

func doAutoDetect(cfg *AIConfig) Provider {
	if cfg != nil && cfg.Provider != "" {
		p, err := ResolveProvider(cfg)
		if err == nil && p != nil {
			return p
		}
	}

	model := detectOllamaModel("http://localhost:11434")
	if model == "" {
		return Noop{}
	}
	return NewOllama("http://localhost:11434", model)
}

func detectOllamaModel(base string) string {
	client := &http.Client{Timeout: 300 * time.Millisecond}
	resp, err := client.Get(base + "/api/tags")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Models) == 0 {
		return ""
	}

	preferred := []string{"llama3.2:latest", "llama3:latest"}
	nameSet := map[string]bool{}
	for _, m := range result.Models {
		nameSet[m.Name] = true
	}
	for _, p := range preferred {
		if nameSet[p] {
			return p
		}
	}
	return result.Models[0].Name
}

// ResetAutoDetect clears the cached provider, used for testing.
func ResetAutoDetect() {
	autoOnce = sync.Once{}
	autoProvider = nil
}
