// cmd/devkit/runner.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/89jobrien/devkit/internal/council"
	"github.com/89jobrien/devkit/internal/loop"
	"github.com/89jobrien/devkit/internal/tools"
	"github.com/anthropics/anthropic-sdk-go"
)

// agentRunner adapts loop.RunAgent (Anthropic) to the Runner interface.
type agentRunner struct {
	client anthropic.Client
}

func newAgentRunner() *agentRunner {
	return &agentRunner{
		client: anthropic.NewClient(), // reads ANTHROPIC_API_KEY from env
	}
}

func (r *agentRunner) Run(ctx context.Context, prompt string, toolNames []string) (string, error) {
	wd, _ := os.Getwd()
	allTools := []tools.Tool{
		tools.ReadTool(wd),
		tools.GlobTool(wd),
		tools.GrepTool(wd),
	}

	var selected []tools.Tool
	if len(toolNames) == 0 {
		selected = allTools
	} else {
		nameSet := make(map[string]bool, len(toolNames))
		for _, n := range toolNames {
			nameSet[n] = true
		}
		for _, t := range allTools {
			if nameSet[t.Definition.OfTool.Name] {
				selected = append(selected, t)
			}
		}
	}

	return loop.RunAgent(ctx, r.client, prompt, selected)
}

// openAIRunner calls the OpenAI chat completions API without tool use.
// It reads OPENAI_API_KEY from env; returns an error if the key is absent.
type openAIRunner struct {
	key    string
	model  string
	client *http.Client
}

func newOpenAIRunner() (*openAIRunner, bool) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return nil, false
	}
	return &openAIRunner{
		key:    key,
		model:  "gpt-4.1",
		client: &http.Client{Timeout: 120 * time.Second},
	}, true
}

func (r *openAIRunner) Run(ctx context.Context, prompt string, _ []string) (string, error) {
	// Strip the tool-use instruction; openAIRunner has no tool support.
	prompt = strings.ReplaceAll(prompt, council.ToolUseInstruction, "")
	body, err := json.Marshal(map[string]any{
		"model":      r.model,
		"max_tokens": 4096,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	})
	if err != nil {
		return "", fmt.Errorf("openai: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+r.key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("openai: read response: %w", err)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return "", fmt.Errorf("openai HTTP %d: authentication error", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		snippet := raw
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		return "", fmt.Errorf("openai HTTP %d: %s", resp.StatusCode, snippet)
	}

	var result struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", errors.New("openai: no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}
