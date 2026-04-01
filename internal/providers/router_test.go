package providers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/providers"
	"github.com/89jobrien/devkit/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openAISimpleServer returns a minimal OpenAI-compatible httptest.Server that
// always responds with the given content string.
func openAISimpleServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":     "chatcmpl-01",
			"object": "chat.completion",
			"choices": []map[string]any{{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": content},
			}},
		})
	}))
}

func TestRouterForTier_AnthropicOnlyKey(t *testing.T) {
	r := providers.NewRouter(providers.RouterConfig{
		AnthropicKey: "ant-key",
	})
	runner := r.For(providers.TierCoding)
	require.NotNil(t, runner)
}

func TestRouterForTier_NoKeysReturnsError(t *testing.T) {
	r := providers.NewRouter(providers.RouterConfig{})
	runner := r.For(providers.TierFast)
	_, err := runner.Run(context.Background(), "hello", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider available")
}

func TestRouterTierForRole(t *testing.T) {
	assert.Equal(t, providers.TierFast, providers.TierForRole("creative-explorer"))
	assert.Equal(t, providers.TierFast, providers.TierForRole("performance-analyst"))
	assert.Equal(t, providers.TierLargeContext, providers.TierForRole("general-analyst"))
	assert.Equal(t, providers.TierBalanced, providers.TierForRole("strict-critic"))
	assert.Equal(t, providers.TierBalanced, providers.TierForRole("unknown-role"))
}

func TestRouterConfigOverride(t *testing.T) {
	r := providers.NewRouter(providers.RouterConfig{
		AnthropicKey: "ant-key",
		Overrides: providers.TierOverrides{
			FastModel: "claude-haiku-4-5-custom",
		},
	})
	assert.NotNil(t, r.For(providers.TierFast))
}

func TestRouterPrimaryProviderReorder(t *testing.T) {
	// Anthropic server that always 500s.
	antSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer antSrv.Close()

	// OpenAI server that succeeds.
	oaiSrv := openAISimpleServer(t, "from openai")
	defer oaiSrv.Close()

	r := providers.NewRouter(providers.RouterConfig{
		AnthropicKey: "ant-key",
		OpenAIKey:    "oai-key",
		Overrides:    providers.TierOverrides{PrimaryProvider: "openai"},
		AnthropicURL: antSrv.URL,
		OpenAIURL:    oaiSrv.URL,
	})
	result, err := r.For(providers.TierBalanced).Run(context.Background(), "hello", nil)
	require.NoError(t, err)
	assert.Equal(t, "from openai", result)
}

func TestRouterFallbackOnError(t *testing.T) {
	// First provider (Anthropic) always errors; second (OpenAI) succeeds.
	antSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer antSrv.Close()

	oaiSrv := openAISimpleServer(t, "fallback result")
	defer oaiSrv.Close()

	r := providers.NewRouter(providers.RouterConfig{
		AnthropicKey: "ant-key",
		OpenAIKey:    "oai-key",
		AnthropicURL: antSrv.URL,
		OpenAIURL:    oaiSrv.URL,
	})
	result, err := r.For(providers.TierBalanced).Run(context.Background(), "hello", nil)
	require.NoError(t, err)
	assert.Equal(t, "fallback result", result)
}

func TestRouterAllProvidersFail(t *testing.T) {
	antSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer antSrv.Close()
	oaiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer oaiSrv.Close()

	r := providers.NewRouter(providers.RouterConfig{
		AnthropicKey: "ant-key",
		OpenAIKey:    "oai-key",
		AnthropicURL: antSrv.URL,
		OpenAIURL:    oaiSrv.URL,
	})
	_, err := r.For(providers.TierBalanced).Run(context.Background(), "hello", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all providers failed")
}

func TestRouterCodingTierExcludesGemini(t *testing.T) {
	// Only Gemini key set — coding tier should have no providers.
	gemSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{}`)
	}))
	defer gemSrv.Close()

	r := providers.NewRouter(providers.RouterConfig{
		GeminiKey: "gem-key",
		GeminiURL: gemSrv.URL,
	})
	_, err := r.For(providers.TierCoding).Run(context.Background(), "hello", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider available")
}

func TestRouterAgentRunnerForSkipsNonAgentProviders(t *testing.T) {
	// Gemini is ChatProvider only; with only Gemini key, AgentRunnerFor should fail.
	gemSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{"content": map[string]any{"parts": []map[string]any{{"text": "hi"}}}}},
		})
	}))
	defer gemSrv.Close()

	r := providers.NewRouter(providers.RouterConfig{
		GeminiKey: "gem-key",
		GeminiURL: gemSrv.URL,
	})
	agentRunner := r.AgentRunnerFor(providers.TierBalanced, []tools.Tool{})
	_, err := agentRunner.Run(context.Background(), "hello", nil)
	assert.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "does not support tool use") ||
			strings.Contains(err.Error(), "no provider available") ||
			strings.Contains(err.Error(), "no agent-capable"),
	)
}
