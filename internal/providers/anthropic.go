// internal/providers/anthropic.go
package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/89jobrien/devkit/internal/tools"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider implements AgentProvider using the Anthropic SDK.
type AnthropicProvider struct {
	client anthropic.Client
	model  string
}

// NewAnthropicProvider constructs an AnthropicProvider.
// baseURL is optional; pass "" to use the default Anthropic endpoint.
// Pass a custom URL only in tests (e.g. httptest server).
func NewAnthropicProvider(apiKey, model, baseURL string) *AnthropicProvider {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &AnthropicProvider{
		client: anthropic.NewClient(opts...),
		model:  model,
	}
}

func (p *AnthropicProvider) Chat(ctx context.Context, prompt string) (string, error) {
	return p.RunAgent(ctx, prompt, nil)
}

func (p *AnthropicProvider) RunAgent(ctx context.Context, prompt string, ts []tools.Tool) (string, error) {
	toolMap := make(map[string]tools.Tool, len(ts))
	for _, t := range ts {
		toolMap[t.Definition.OfTool.Name] = t
	}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
	}

	for {
		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(p.model),
			MaxTokens: 8096,
			Messages:  messages,
		}
		if len(ts) > 0 {
			params.Tools = tools.Definitions(ts)
		}

		resp, err := p.client.Messages.New(ctx, params)
		if err != nil {
			return "", fmt.Errorf("anthropic: messages.New: %w", err)
		}
		messages = append(messages, resp.ToParam())

		if resp.StopReason == "end_turn" {
			var sb strings.Builder
			for _, block := range resp.Content {
				if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
					sb.WriteString(tb.Text)
				}
			}
			return sb.String(), nil
		}

		var toolResults []anthropic.ContentBlockParamUnion
		for _, block := range resp.Content {
			tu, ok := block.AsAny().(anthropic.ToolUseBlock)
			if !ok {
				continue
			}
			t, found := toolMap[tu.Name]
			if !found {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(tu.ID, fmt.Sprintf("unknown tool: %s", tu.Name), true))
				continue
			}
			result, err := t.Handler.Handle(ctx, json.RawMessage(tu.JSON.Input.Raw()))
			if err != nil {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(tu.ID, err.Error(), true))
			} else {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(tu.ID, result, false))
			}
		}
		if len(toolResults) > 0 {
			messages = append(messages, anthropic.NewUserMessage(toolResults...))
		}
	}
}
