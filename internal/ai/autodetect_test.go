package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectOllamaModel_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/tags", r.URL.Path)
		resp := map[string]interface{}{
			"models": []map[string]string{
				{"name": "mistral:latest"},
				{"name": "llama3:latest"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	model := detectOllamaModel(srv.URL)
	assert.Equal(t, "llama3:latest", model, "should prefer llama3:latest")
}

func TestDetectOllamaModel_PreferLlama32(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"models": []map[string]string{
				{"name": "llama3:latest"},
				{"name": "llama3.2:latest"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	model := detectOllamaModel(srv.URL)
	assert.Equal(t, "llama3.2:latest", model)
}

func TestDetectOllamaModel_FallbackToFirst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"models": []map[string]string{
				{"name": "custom-model:v1"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	model := detectOllamaModel(srv.URL)
	assert.Equal(t, "custom-model:v1", model)
}

func TestDetectOllamaModel_Unreachable(t *testing.T) {
	model := detectOllamaModel("http://127.0.0.1:1")
	assert.Empty(t, model)
}

func TestDetectOllamaModel_EmptyModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{"models": []map[string]string{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	model := detectOllamaModel(srv.URL)
	assert.Empty(t, model)
}

func TestAutoDetect_ExplicitConfigTakesPrecedence(t *testing.T) {
	ResetAutoDetect()
	defer ResetAutoDetect()

	cfg := &AIConfig{
		Provider:  "ollama",
		Model:     "custom:v2",
		BaseURL:   "http://localhost:11434",
	}

	p := AutoDetect(cfg)
	require.NotNil(t, p)

	ollama, ok := p.(*Ollama)
	require.True(t, ok, "should return Ollama provider")
	assert.Equal(t, "custom:v2", ollama.model)
}

func TestAutoDetect_NoConfig_NoOllama_ReturnsNoop(t *testing.T) {
	ResetAutoDetect()
	defer ResetAutoDetect()

	p := AutoDetect(nil)
	_, ok := p.(Noop)
	assert.True(t, ok, "should return Noop when no config and no Ollama")
}
