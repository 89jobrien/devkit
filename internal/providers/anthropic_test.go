package providers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/89jobrien/devkit/internal/providers"
	"github.com/89jobrien/devkit/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_01",
			"type":        "message",
			"role":        "assistant",
			"stop_reason": "end_turn",
			"content": []map[string]any{
				{"type": "text", "text": "hello from anthropic"},
			},
			"model": "claude-sonnet-4-6",
			"usage": map[string]int{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	p := providers.NewAnthropicProvider("test-key", providers.ModelAnthropicBalanced, srv.URL)
	result, err := p.Chat(context.Background(), "say hello")
	require.NoError(t, err)
	assert.Equal(t, "hello from anthropic", result)
}

func TestAnthropicRunAgent_EndTurn(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_01",
			"type":        "message",
			"role":        "assistant",
			"stop_reason": "end_turn",
			"content":     []map[string]any{{"type": "text", "text": "done"}},
			"model":       "claude-sonnet-4-6",
			"usage":       map[string]int{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	p := providers.NewAnthropicProvider("test-key", providers.ModelAnthropicCoding, srv.URL)
	result, err := p.RunAgent(context.Background(), "do work", []tools.Tool{})
	require.NoError(t, err)
	assert.Equal(t, "done", result)
	assert.Equal(t, 1, calls)
}
