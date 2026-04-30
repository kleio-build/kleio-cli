package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Anthropic implements Provider for the Anthropic Messages API.
type Anthropic struct {
	apiKey string
	model  string
	client *http.Client
}

func NewAnthropic(apiKey, model string) *Anthropic {
	return &Anthropic{apiKey: apiKey, model: model, client: &http.Client{}}
}

func (a *Anthropic) Available() bool { return a.apiKey != "" }

func (a *Anthropic) Complete(ctx context.Context, prompt string) (string, error) {
	body := map[string]interface{}{
		"model":      a.model,
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic: %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Content) == 0 {
		return "", nil
	}
	return result.Content[0].Text, nil
}

// Embed is not natively supported by Anthropic; returns nil.
func (a *Anthropic) Embed(_ context.Context, _ string) ([]float64, error) {
	return nil, nil
}
