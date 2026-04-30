package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Ollama implements Provider for a local Ollama server, enabling fully
// offline LLM usage.
type Ollama struct {
	base   string
	model  string
	client *http.Client
}

func NewOllama(baseURL, model string) *Ollama {
	return &Ollama{base: baseURL, model: model, client: &http.Client{}}
}

func (o *Ollama) Available() bool { return true }

func (o *Ollama) Complete(ctx context.Context, prompt string) (string, error) {
	body := map[string]interface{}{
		"model":  o.model,
		"prompt": prompt,
		"stream": false,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.base+"/api/generate", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama not reachable at %s: %w", o.base, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama: %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Response, nil
}

func (o *Ollama) Embed(ctx context.Context, text string) ([]float64, error) {
	body := map[string]interface{}{
		"model":  o.model,
		"prompt": text,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.base+"/api/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed: %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}
