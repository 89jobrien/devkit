package providers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/89jobrien/devkit/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]any{{"text": "hello from gemini"}},
				},
			}},
		})
	}))
	defer srv.Close()

	p := providers.NewGeminiProvider("test-key", providers.ModelGeminiLargeContext, srv.URL)
	result, err := p.Chat(context.Background(), "say hello")
	require.NoError(t, err)
	assert.Equal(t, "hello from gemini", result)
}

func TestGeminiChat_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"quota exceeded"}}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := providers.NewGeminiProvider("test-key", providers.ModelGeminiFast, srv.URL)
	_, err := p.Chat(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
}

func TestGeminiChat_EmptyCandidates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"candidates": []any{}})
	}))
	defer srv.Close()

	p := providers.NewGeminiProvider("test-key", providers.ModelGeminiFast, srv.URL)
	_, err := p.Chat(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no candidates")
}

func TestGeminiSatisfiesChatProvider(t *testing.T) {
	p := providers.NewGeminiProvider("key", providers.ModelGeminiFast, "")
	var _ providers.ChatProvider = p
}
