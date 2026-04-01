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

var _ providers.AgentProvider = (*providers.OpenAIProvider)(nil)

func openAIResponse(content string) map[string]any {
	return map[string]any{
		"id":      "chatcmpl-01",
		"object":  "chat.completion",
		"choices": []map[string]any{{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}},
		"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 5},
	}
}

func TestOpenAIChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIResponse("hello from openai"))
	}))
	defer srv.Close()

	p := providers.NewOpenAIProvider("test-key", providers.ModelOpenAIBalanced, srv.URL)
	result, err := p.Chat(context.Background(), "say hello")
	require.NoError(t, err)
	assert.Equal(t, "hello from openai", result)
}

func TestOpenAIChat_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"rate limited"}}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := providers.NewOpenAIProvider("test-key", providers.ModelOpenAIBalanced, srv.URL)
	_, err := p.Chat(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
}

func TestOpenAIRunAgent_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	}))
	defer srv.Close()

	p := providers.NewOpenAIProvider("test-key", providers.ModelOpenAIBalanced, srv.URL)
	_, err := p.Chat(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

func TestOpenAIRunAgent_UnknownTool(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"role": "assistant", "content": nil,
						"tool_calls": []map[string]any{{
							"id": "call_01", "type": "function",
							"function": map[string]any{"name": "nonexistent", "arguments": `{}`},
						}},
					},
				}},
			})
		} else {
			json.NewEncoder(w).Encode(openAIResponse("handled unknown tool"))
		}
	}))
	defer srv.Close()

	p := providers.NewOpenAIProvider("test-key", providers.ModelOpenAICoding, srv.URL)
	result, err := p.RunAgent(context.Background(), "use tool", []tools.Tool{})
	require.NoError(t, err)
	assert.Equal(t, "handled unknown tool", result)
	assert.Equal(t, 2, calls, "unknown tool result should be sent back to model")
}

func TestOpenAIRunAgent_ToolCall(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			// First call: model requests a tool.
			json.NewEncoder(w).Encode(map[string]any{
				"id":     "chatcmpl-01",
				"object": "chat.completion",
				"choices": []map[string]any{{
					"index":         0,
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"role":    "assistant",
						"content": nil,
						"tool_calls": []map[string]any{{
							"id":   "call_01",
							"type": "function",
							"function": map[string]any{
								"name":      "Read",
								"arguments": `{"path":"README.md"}`,
							},
						}},
					},
				}},
			})
		} else {
			// Second call: model finishes.
			json.NewEncoder(w).Encode(openAIResponse("done after tool"))
		}
	}))
	defer srv.Close()

	readTool := tools.ReadTool(t.TempDir())
	p := providers.NewOpenAIProvider("test-key", providers.ModelOpenAICoding, srv.URL)
	result, err := p.RunAgent(context.Background(), "read the readme", []tools.Tool{readTool})
	require.NoError(t, err)
	assert.Equal(t, "done after tool", result)
	assert.Equal(t, 2, calls)
}
