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

// bearerTransport injects the Authorization header via RoundTripper so the key
// never appears on the *http.Request visible to error handlers or loggers.
type bearerTransport struct {
	key  string
	base http.RoundTripper
}

func (t *bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("Authorization", "Bearer "+t.key)
	return t.base.RoundTrip(r2)
}

// openAIRunner calls the OpenAI chat completions API without tool use.
// It reads OPENAI_API_KEY from env; returns false if the key is absent.
type openAIRunner struct {
	model  string
	client *http.Client
}

func newOpenAIRunner() (*openAIRunner, bool) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return nil, false
	}
	return &openAIRunner{
		model: "gpt-4.1",
		client: &http.Client{
			Timeout:   120 * time.Second,
			Transport: &bearerTransport{key: key, base: http.DefaultTransport.(*http.Transport).Clone()},
		},
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
	if resp.StatusCode >= 400 {
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return "", fmt.Errorf("openai HTTP %d: authentication error — check OPENAI_API_KEY", resp.StatusCode)
		}
		s := strings.ToValidUTF8(string(raw), "")
		if len(s) > 512 {
			s = s[:512]
		}
		return "", fmt.Errorf("openai HTTP %d: %s", resp.StatusCode, s)
	}

	var result struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("openai: decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", errors.New("openai: no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}
