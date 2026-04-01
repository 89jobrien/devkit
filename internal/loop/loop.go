// internal/loop/loop.go
package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/89jobrien/devkit/internal/tools"
	"github.com/anthropics/anthropic-sdk-go"
)

const (
	defaultModel     = anthropic.ModelClaudeSonnet4_6
	defaultMaxTokens = 8096
)

// RunAgent runs a tool-use loop against the Anthropic Messages API until
// stop_reason is "end_turn". Returns the concatenated text of the final response.
func RunAgent(ctx context.Context, client anthropic.Client, prompt string, ts []tools.Tool) (string, error) {
	toolMap := make(map[string]tools.Tool, len(ts))
	for _, t := range ts {
		toolMap[t.Definition.OfTool.Name] = t
	}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
	}

	for {
		params := anthropic.MessageNewParams{
			Model:     defaultModel,
			MaxTokens: defaultMaxTokens,
			Messages:  messages,
		}
		if len(ts) > 0 {
			params.Tools = tools.Definitions(ts)
		}

		resp, err := client.Messages.New(ctx, params)
		if err != nil {
			return "", fmt.Errorf("messages.New: %w", err)
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

		// Handle tool_use stop reason
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

// RunAgentLoop runs a tool-use agent loop using any AgentProvider.
// This is the provider-agnostic counterpart to RunAgent (Anthropic-SDK-specific).
func RunAgentLoop(ctx context.Context, p interface {
	RunAgent(ctx context.Context, prompt string, ts []tools.Tool) (string, error)
}, prompt string, ts []tools.Tool) (string, error) {
	return p.RunAgent(ctx, prompt, ts)
}
