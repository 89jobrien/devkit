// internal/chain/synthesis_test.go
package chain_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/89jobrien/devkit/internal/chain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynthesisRunnerCallsOpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":     "chatcmpl-01",
			"object": "chat.completion",
			"choices": []map[string]any{{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": "synthesis output"},
			}},
		})
	}))
	defer srv.Close()

	runner := chain.NewSynthesisRunner("oai-key", srv.URL)
	prior := []chain.Result{
		{Stage: "council", Output: "council output"},
		{},                                                  // skipped slot
		{Stage: "ticket", Output: "ticket output", Err: nil},
	}
	result := runner.Run(context.Background(), prior)
	require.NoError(t, result.Err)
	assert.Equal(t, "synthesis", result.Stage)
	assert.Equal(t, "synthesis output", result.Output)
	p, ok := result.Payload.(*chain.SynthesisPayload)
	assert.True(t, ok)
	assert.Equal(t, "synthesis output", p.Summary)
}

func TestSynthesisIncludesSkippedSlotsAsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture the request body to inspect the prompt.
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		msgs := body["messages"].([]any)
		userMsg := msgs[len(msgs)-1].(map[string]any)["content"].(string)
		// The prompt must mention skipped stages.
		assert.Contains(t, userMsg, "[skipped]")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "x", "object": "chat.completion",
			"choices": []map[string]any{{"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": "ok"}}},
		})
	}))
	defer srv.Close()

	runner := chain.NewSynthesisRunner("oai-key", srv.URL)
	prior := []chain.Result{
		{Stage: "council", Output: "text"},
		{}, // skipped
	}
	runner.Run(context.Background(), prior)
}
