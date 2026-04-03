// internal/providers/openai.go
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/89jobrien/devkit/internal/infra/tools"
)

// OpenAIProvider implements AgentProvider using the OpenAI chat completions API
// with function-calling for tool use.
type OpenAIProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewOpenAIProvider constructs an OpenAIProvider.
// baseURL defaults to "https://api.openai.com" if empty; override in tests.
func NewOpenAIProvider(apiKey, model, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAIProvider) Chat(ctx context.Context, prompt string) (string, error) {
	return p.RunAgent(ctx, prompt, nil)
}

// openAIMessage is an element of the messages array sent to OpenAI.
type openAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"`           // string or nil
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// openAITool is the function schema sent in the tools array.
type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

func toolsToOpenAI(ts []tools.Tool) []openAITool {
	out := make([]openAITool, 0, len(ts))
	for _, t := range ts {
		def := t.Definition.OfTool
		if def == nil {
			continue
		}
		var ot openAITool
		ot.Type = "function"
		ot.Function.Name = def.Name
		if def.Description.Valid() {
			ot.Function.Description = def.Description.Value
		}
		required := def.InputSchema.Required
		if required == nil {
			required = []string{}
		}
		ot.Function.Parameters = map[string]any{
			"type":       "object",
			"properties": def.InputSchema.Properties,
			"required":   required,
		}
		out = append(out, ot)
	}
	return out
}

func (p *OpenAIProvider) post(ctx context.Context, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("openai: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openai HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

func (p *OpenAIProvider) RunAgent(ctx context.Context, prompt string, ts []tools.Tool) (string, error) {
	toolMap := make(map[string]tools.Tool, len(ts))
	for _, t := range ts {
		if t.Definition.OfTool != nil {
			toolMap[t.Definition.OfTool.Name] = t
		}
	}

	messages := []openAIMessage{{Role: "user", Content: prompt}}

	for {
		reqBody := map[string]any{
			"model":                  p.model,
			"max_completion_tokens": 8096,
			"messages":              messages,
		}
		if len(ts) > 0 {
			reqBody["tools"] = toolsToOpenAI(ts)
		}

		raw, err := p.post(ctx, reqBody)
		if err != nil {
			return "", err
		}

		var resp struct {
			Choices []struct {
				FinishReason string        `json:"finish_reason"`
				Message      openAIMessage `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return "", fmt.Errorf("openai: decode response: %w", err)
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("openai: no choices in response")
		}

		choice := resp.Choices[0]
		messages = append(messages, choice.Message)

		if choice.FinishReason == "stop" || choice.FinishReason == "end_turn" {
			if s, ok := choice.Message.Content.(string); ok {
				return s, nil
			}
			return "", nil
		}

		if choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) == 0 {
			// Unexpected finish reason — return whatever content exists.
			if s, ok := choice.Message.Content.(string); ok {
				return s, nil
			}
			return "", nil
		}

		// Dispatch tool calls and inject results.
		for _, tc := range choice.Message.ToolCalls {
			t, found := toolMap[tc.Function.Name]
			var resultContent string
			if !found {
				resultContent = fmt.Sprintf("unknown tool: %s", tc.Function.Name)
			} else {
				res, err := t.Handler.Handle(ctx, json.RawMessage(tc.Function.Arguments))
				if err != nil {
					resultContent = err.Error()
				} else {
					resultContent = res
				}
			}
			messages = append(messages, openAIMessage{
				Role:       "tool",
				Content:    resultContent,
				ToolCallID: tc.ID,
			})
		}
	}
}
