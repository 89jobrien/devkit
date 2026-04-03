// internal/loop/loop_test.go
package loop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/89jobrien/devkit/internal/ai/loop"
	"github.com/89jobrien/devkit/internal/infra/tools"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockEndTurnResponse(text string) []byte {
	resp := map[string]any{
		"id":    "msg_test",
		"type":  "message",
		"role":  "assistant",
		"model": "claude-sonnet-4-6",
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 10, "output_tokens": 20},
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestRunAgentEndTurn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockEndTurnResponse("Hello, world!"))
	}))
	defer srv.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(srv.URL),
	)

	result, err := loop.RunAgent(context.Background(), client, "say hello", nil)
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", result)
}

func TestRunAgentToolUse(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		calls++
		if calls == 1 {
			resp := map[string]any{
				"id": "msg_1", "type": "message", "role": "assistant",
				"model": "claude-sonnet-4-6",
				"content": []map[string]any{{
					"type":  "tool_use",
					"id":    "tu_1",
					"name":  "echo",
					"input": map[string]string{"text": "ping"},
				}},
				"stop_reason": "tool_use",
				"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
			}
			b, _ := json.Marshal(resp)
			w.Write(b)
		} else {
			w.Write(mockEndTurnResponse("done"))
		}
	}))
	defer srv.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(srv.URL),
	)

	echoTool := tools.Tool{
		Definition: anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{Name: "echo"}},
		Handler: tools.HandlerFunc(func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct{ Text string `json:"text"` }
			json.Unmarshal(input, &args)
			return fmt.Sprintf("pong: %s", args.Text), nil
		}),
	}

	result, err := loop.RunAgent(context.Background(), client, "use the echo tool", []tools.Tool{echoTool})
	require.NoError(t, err)
	assert.Equal(t, "done", result)
	assert.Equal(t, 2, calls)
}


