# Provider Fallback & Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `internal/providers` package that implements a generic tool-use loop and semantic tier-based routing across Anthropic, OpenAI, and Gemini, replacing the current Anthropic-only `agentRunner` and ad-hoc `openAIRunner`.

**Architecture:** A new `internal/providers` package defines `ChatProvider` and `AgentProvider` interfaces plus a `Router` that maps tiers to provider chains. A generic `RunAgentLoop` in `internal/loop` replaces the Anthropic-SDK-specific loop. `cmd/devkit/runner.go` is replaced by thin wiring that constructs providers from env keys and delegates to the router.

**Tech Stack:** Go 1.26, `github.com/anthropics/anthropic-sdk-go`, `net/http`, `encoding/json`, `net/http/httptest` for tests.

---

## File Map

| File                                   | Action    | Responsibility                                                                                  |
| -------------------------------------- | --------- | ----------------------------------------------------------------------------------------------- |
| `internal/providers/provider.go`       | Create    | `ChatProvider`, `AgentProvider` interfaces; `Tier` constants; `ToolCall`/`Message` shared types |
| `internal/providers/models.go`         | Create    | Model ID constants per tier per provider                                                        |
| `internal/providers/anthropic.go`      | Create    | Anthropic `AgentProvider` (wraps SDK)                                                           |
| `internal/providers/openai.go`         | Create    | OpenAI `AgentProvider` (function-calling loop via raw HTTP)                                     |
| `internal/providers/gemini.go`         | Create    | Gemini `ChatProvider` (large context, no tool use)                                              |
| `internal/providers/router.go`         | Create    | `Router`: tier→provider chain selection, fallback logic, role→tier map                          |
| `internal/providers/anthropic_test.go` | Create    | httptest-based unit tests for Anthropic provider                                                |
| `internal/providers/openai_test.go`    | Create    | httptest-based unit tests for OpenAI provider                                                   |
| `internal/providers/gemini_test.go`    | Create    | httptest-based unit tests for Gemini provider                                                   |
| `internal/providers/router_test.go`    | Create    | Unit tests for tier routing, fallback, role mapping                                             |
| `internal/loop/loop.go`                | Modify    | Add `RunAgentLoop(ctx, AgentProvider, prompt, tools)` alongside existing `RunAgent`             |
| `internal/loop/loop_test.go`           | Modify    | Add tests for `RunAgentLoop` using stub `AgentProvider`                                         |
| `cmd/devkit/runner.go`                 | Replace   | Wire `Router` from env keys; implement `Runner` adapter over `AgentProvider`/`ChatProvider`     |
| `cmd/devkit/config.go`                 | Modify    | Add `[providers]` section to `Config` struct                                                    |
| `cmd/devkit/main.go`                   | Modify    | Use `Router.For(tier)` for all commands; map council roles to tiers                             |
| `cmd/ci-agent/providers.go`            | No change | Already has its own `askWithFallback`; leave as-is                                              |

---

## Task 1: Define shared interfaces and types

**Files:**

- Create: `internal/providers/provider.go`

- [x] **Step 1: Write the failing test (interfaces compile check)**

Create `internal/providers/provider_test.go`:

```go
package providers_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/providers"
	"github.com/89jobrien/devkit/internal/tools"
)

// Compile-time checks: stub types must satisfy the interfaces.
type stubChat struct{}

func (s stubChat) Chat(_ context.Context, _ string) (string, error) { return "ok", nil }

type stubAgent struct{ stubChat }

func (s stubAgent) RunAgent(_ context.Context, _ string, _ []tools.Tool) (string, error) {
	return "ok", nil
}

func TestInterfacesCompile(t *testing.T) {
	var _ providers.ChatProvider = stubChat{}
	var _ providers.AgentProvider = stubAgent{}
}
```

- [x] **Step 2: Run to confirm it fails**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/...
```

Expected: `no Go files in .../internal/providers` or compile error.

- [x] **Step 3: Implement `provider.go`**

```go
// internal/providers/provider.go
package providers

import (
	"context"

	"github.com/89jobrien/devkit/internal/tools"
)

// ChatProvider is a single-turn LLM completion with no tool use.
type ChatProvider interface {
	Chat(ctx context.Context, prompt string) (string, error)
}

// AgentProvider supports multi-turn tool-use agentic loops.
type AgentProvider interface {
	ChatProvider
	RunAgent(ctx context.Context, prompt string, ts []tools.Tool) (string, error)
}

// Tier classifies the nature of a task for provider selection.
type Tier string

const (
	TierFast         Tier = "fast"          // cheap, exploratory, mapping
	TierBalanced     Tier = "balanced"      // reasoning, synthesis
	TierLargeContext Tier = "large-context" // full diff/repo ingestion
	TierCoding       Tier = "coding"        // agentic tool use, implementation
)
```

- [x] **Step 4: Run test to confirm it passes**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/joe/dev/devkit && git add internal/providers/ && git commit -m "feat(providers): add ChatProvider/AgentProvider interfaces and Tier constants"
```

---

## Task 2: Model ID constants

**Files:**

- Create: `internal/providers/models.go`

- [x] **Step 1: Write the test**

Add to `internal/providers/provider_test.go`:

```go
func TestModelConstants(t *testing.T) {
	// Verify non-empty strings only — exact values checked by inspection.
	assert.NotEmpty(t, providers.ModelAnthropicFast)
	assert.NotEmpty(t, providers.ModelAnthropicBalanced)
	assert.NotEmpty(t, providers.ModelAnthropicLargeContext)
	assert.NotEmpty(t, providers.ModelAnthropicCoding)
	assert.NotEmpty(t, providers.ModelOpenAIFast)
	assert.NotEmpty(t, providers.ModelOpenAIBalanced)
	assert.NotEmpty(t, providers.ModelOpenAICoding)
	assert.NotEmpty(t, providers.ModelGeminiFast)
	assert.NotEmpty(t, providers.ModelGeminiBalanced)
	assert.NotEmpty(t, providers.ModelGeminiLargeContext)
}
```

Add import `"github.com/stretchr/testify/assert"` to the test file.

- [x] **Step 2: Run to confirm it fails**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/...
```

Expected: compile error — `ModelAnthropicFast` undefined.

- [x] **Step 3: Implement `models.go`**

```go
// internal/providers/models.go
package providers

// Anthropic model IDs by tier.
const (
	ModelAnthropicFast         = "claude-haiku-4-5"
	ModelAnthropicBalanced     = "claude-sonnet-4-6"
	ModelAnthropicLargeContext = "claude-sonnet-4-5" // 1M context window
	ModelAnthropicCoding       = "claude-sonnet-4-6"
)

// OpenAI model IDs by tier (gpt-5.4 series).
const (
	ModelOpenAIFast     = "gpt-5.4-mini"
	ModelOpenAIBalanced = "gpt-5.4"
	ModelOpenAICoding   = "gpt-5.4"
)

// Gemini model IDs by tier (gemini-3 series).
const (
	ModelGeminiFast         = "gemini-3-flash-preview"
	ModelGeminiBalanced     = "gemini-3-pro-preview"
	ModelGeminiLargeContext = "gemini-3-pro-preview" // 1M+ context
)
```

- [x] **Step 4: Run test to confirm it passes**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/joe/dev/devkit && git add internal/providers/ && git commit -m "feat(providers): add model ID constants for all tiers"
```

---

## Task 3: Anthropic provider

**Files:**

- Create: `internal/providers/anthropic.go`
- Create: `internal/providers/anthropic_test.go`

- [x] **Step 1: Write the failing test**

Create `internal/providers/anthropic_test.go`:

```go
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
			"id":         "msg_01",
			"type":       "message",
			"role":       "assistant",
			"stop_reason": "end_turn",
			"content": []map[string]any{
				{"type": "text", "text": "hello from anthropic"},
			},
			"model":   "claude-sonnet-4-6",
			"usage":   map[string]int{"input_tokens": 10, "output_tokens": 5},
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
```

- [x] **Step 2: Run to confirm it fails**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/... -run TestAnthropic
```

Expected: compile error — `NewAnthropicProvider` undefined.

- [x] **Step 3: Implement `anthropic.go`**

```go
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
	client  anthropic.Client
	model   string
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
```

- [x] **Step 4: Run tests to confirm they pass**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/... -run TestAnthropic -v
```

Expected: PASS both tests.

- [ ] **Step 5: Commit**

```bash
cd /Users/joe/dev/devkit && git add internal/providers/ && git commit -m "feat(providers): add AnthropicProvider with tool-use loop"
```

---

## Task 4: OpenAI provider

**Files:**

- Create: `internal/providers/openai.go`
- Create: `internal/providers/openai_test.go`

- [x] **Step 1: Write the failing test**

Create `internal/providers/openai_test.go`:

```go
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
					"index":        0,
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
```

- [x] **Step 2: Run to confirm it fails**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/... -run TestOpenAI
```

Expected: compile error — `NewOpenAIProvider` undefined.

- [x] **Step 3: Implement `openai.go`**

```go
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

	"github.com/89jobrien/devkit/internal/tools"
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
	Role       string          `json:"role"`
	Content    any             `json:"content"`           // string or nil
	ToolCallID string          `json:"tool_call_id,omitempty"`
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
		if def.Description != nil {
			ot.Function.Description = *def.Description
		}
		ot.Function.Parameters = map[string]any{
			"type":       "object",
			"properties": def.InputSchema.Properties,
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
			"model":      p.model,
			"max_tokens": 8096,
			"messages":   messages,
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
```

- [x] **Step 4: Run tests to confirm they pass**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/... -run TestOpenAI -v
```

Expected: PASS both tests.

- [ ] **Step 5: Commit**

```bash
cd /Users/joe/dev/devkit && git add internal/providers/ && git commit -m "feat(providers): add OpenAIProvider with function-calling tool loop"
```

---

## Task 5: Gemini provider

**Files:**

- Create: `internal/providers/gemini.go`
- Create: `internal/providers/gemini_test.go`

- [x] **Step 1: Write the failing test**

Create `internal/providers/gemini_test.go`:

```go
package providers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/89jobrien/devkit/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]any{{"text": "hello from gemini"}},
				},
			}},
		})
	}))
	defer srv.Close()

	p := providers.NewGeminiProvider("test-key", providers.ModelGeminiLargeContext, srv.URL)
	result, err := p.Chat(context.Background(), "say hello")
	require.NoError(t, err)
	assert.Equal(t, "hello from gemini", result)
}

func TestGeminiSatisfiesChatProvider(t *testing.T) {
	p := providers.NewGeminiProvider("key", providers.ModelGeminiFast, "")
	var _ providers.ChatProvider = p
}
```

- [x] **Step 2: Run to confirm it fails**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/... -run TestGemini
```

Expected: compile error — `NewGeminiProvider` undefined.

- [x] **Step 3: Implement `gemini.go`**

```go
// internal/providers/gemini.go
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
)

// GeminiProvider implements ChatProvider using the Gemini generateContent API.
// Gemini is used for large-context ingestion; tool use is not supported here.
type GeminiProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewGeminiProvider constructs a GeminiProvider.
// baseURL defaults to "https://generativelanguage.googleapis.com" if empty.
func NewGeminiProvider(apiKey, model, baseURL string) *GeminiProvider {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	return &GeminiProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *GeminiProvider) Chat(ctx context.Context, prompt string) (string, error) {
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", p.baseURL, p.model)
	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{{
			"parts": []map[string]any{{"text": prompt}},
		}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("gemini: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gemini HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("gemini: decode response: %w", err)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: no candidates in response")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}
```

- [x] **Step 4: Run tests to confirm they pass**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/... -run TestGemini -v
```

Expected: PASS both tests.

- [ ] **Step 5: Commit**

```bash
cd /Users/joe/dev/devkit && git add internal/providers/ && git commit -m "feat(providers): add GeminiProvider for large-context chat"
```

---

## Task 6: Router — tier-to-provider chain and role mapping

**Files:**

- Create: `internal/providers/router.go`
- Create: `internal/providers/router_test.go`

- [x] **Step 1: Write the failing tests**

Create `internal/providers/router_test.go`:

```go
package providers_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	// When .devkit.toml sets fast_model, the router uses it.
	r := providers.NewRouter(providers.RouterConfig{
		AnthropicKey: "ant-key",
		Overrides: providers.TierOverrides{
			FastModel: "claude-haiku-4-5-custom",
		},
	})
	// Just verify it constructs without panic — model used at call time.
	assert.NotNil(t, r.For(providers.TierFast))
}
```

- [x] **Step 2: Run to confirm it fails**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/... -run TestRouter
```

Expected: compile error — `NewRouter` undefined.

- [x] **Step 3: Implement `router.go`**

```go
// internal/providers/router.go
package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/89jobrien/devkit/internal/council"
	"github.com/89jobrien/devkit/internal/tools"
)

// TierOverrides allows .devkit.toml to pin specific model IDs per tier.
type TierOverrides struct {
	PrimaryProvider string // "anthropic" | "openai" | "gemini"
	FastModel       string
	BalancedModel   string
	LargeContextModel string
	CodingModel     string
}

// RouterConfig holds API keys and optional overrides.
type RouterConfig struct {
	AnthropicKey string
	OpenAIKey    string
	GeminiKey    string
	Overrides    TierOverrides
}

// Router selects and chains providers for a given Tier.
type Router struct {
	cfg RouterConfig
}

// NewRouter constructs a Router from the given config.
func NewRouter(cfg RouterConfig) *Router {
	return &Router{cfg: cfg}
}

// roleTierMap maps council role keys to tiers.
var roleTierMap = map[string]Tier{
	"creative-explorer":   TierFast,
	"performance-analyst": TierFast,
	"general-analyst":     TierLargeContext,
	"security-reviewer":   TierBalanced,
	"strict-critic":       TierBalanced,
}

// TierForRole returns the appropriate Tier for a council role key.
// Unknown roles default to TierBalanced.
func TierForRole(role string) Tier {
	if t, ok := roleTierMap[role]; ok {
		return t
	}
	return TierBalanced
}

// For returns a council.Runner that tries the provider chain for the given tier.
// Providers with missing API keys are skipped. If no provider has a key,
// the runner returns an error on first call.
func (r *Router) For(tier Tier) council.Runner {
	return council.RunnerFunc(func(ctx context.Context, prompt string, toolNames []string) (string, error) {
		chain := r.chainFor(tier)
		if len(chain) == 0 {
			return "", errors.New("no provider available for tier " + string(tier) + ": set ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY")
		}
		var errs []string
		for _, entry := range chain {
			var result string
			var err error
			if ap, ok := entry.provider.(AgentProvider); ok && len(toolNames) > 0 {
				// Tool use requested — must use AgentProvider.
				result, err = ap.RunAgent(ctx, prompt, nil) // tools resolved by caller
			} else {
				result, err = entry.provider.Chat(ctx, prompt)
			}
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", entry.name, err))
				continue
			}
			return result, nil
		}
		return "", fmt.Errorf("all providers failed for tier %s: %s", tier, joinErrs(errs))
	})
}

// AgentRunnerFor returns a council.Runner that passes tools through to the provider.
// Use this for commands that require tool use (diagnose, review, meta).
func (r *Router) AgentRunnerFor(tier Tier, ts []tools.Tool) council.Runner {
	return council.RunnerFunc(func(ctx context.Context, prompt string, _ []string) (string, error) {
		chain := r.chainFor(tier)
		if len(chain) == 0 {
			return "", errors.New("no provider available for tier " + string(tier))
		}
		var errs []string
		for _, entry := range chain {
			if ap, ok := entry.provider.(AgentProvider); ok {
				result, err := ap.RunAgent(ctx, prompt, ts)
				if err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", entry.name, err))
					continue
				}
				return result, nil
			}
			// Provider does not support tool use — skip for coding tier.
			errs = append(errs, fmt.Sprintf("%s: does not support tool use", entry.name))
		}
		return "", fmt.Errorf("no agent-capable provider available for tier %s: %s", tier, joinErrs(errs))
	})
}

type providerEntry struct {
	name     string
	provider ChatProvider
}

func (r *Router) chainFor(tier Tier) []providerEntry {
	var chain []providerEntry

	antModel, oaiModel, gemModel := r.modelsForTier(tier)

	// Anthropic first (primary for coding/balanced; supports tool use).
	if r.cfg.AnthropicKey != "" {
		chain = append(chain, providerEntry{
			name:     "anthropic/" + antModel,
			provider: NewAnthropicProvider(r.cfg.AnthropicKey, antModel, ""),
		})
	}
	// OpenAI second (supports tool use).
	if r.cfg.OpenAIKey != "" && oaiModel != "" {
		chain = append(chain, providerEntry{
			name:     "openai/" + oaiModel,
			provider: NewOpenAIProvider(r.cfg.OpenAIKey, oaiModel, ""),
		})
	}
	// Gemini last (large context / fast; no tool use).
	if r.cfg.GeminiKey != "" && gemModel != "" {
		chain = append(chain, providerEntry{
			name:     "gemini/" + gemModel,
			provider: NewGeminiProvider(r.cfg.GeminiKey, gemModel, ""),
		})
	}

	// If PrimaryProvider override is set, reorder to put that provider first.
	if r.cfg.Overrides.PrimaryProvider != "" {
		chain = reorderChain(chain, r.cfg.Overrides.PrimaryProvider)
	}
	return chain
}

func (r *Router) modelsForTier(tier Tier) (ant, oai, gem string) {
	ov := r.cfg.Overrides
	switch tier {
	case TierFast:
		ant = orDefault(ov.FastModel, ModelAnthropicFast)
		oai = ModelOpenAIFast
		gem = ModelGeminiFast
	case TierLargeContext:
		ant = orDefault(ov.LargeContextModel, ModelAnthropicLargeContext)
		oai = ModelOpenAIBalanced
		gem = ModelGeminiLargeContext
	case TierCoding:
		ant = orDefault(ov.CodingModel, ModelAnthropicCoding)
		oai = ModelOpenAICoding
		gem = "" // Gemini excluded from coding tier (no tool use)
	default: // TierBalanced
		ant = orDefault(ov.BalancedModel, ModelAnthropicBalanced)
		oai = ModelOpenAIBalanced
		gem = ModelGeminiBalanced
	}
	return
}

func orDefault(override, def string) string {
	if override != "" {
		return override
	}
	return def
}

func reorderChain(chain []providerEntry, primary string) []providerEntry {
	var first, rest []providerEntry
	for _, e := range chain {
		if len(e.name) >= len(primary) && e.name[:len(primary)] == primary {
			first = append(first, e)
		} else {
			rest = append(rest, e)
		}
	}
	return append(first, rest...)
}

func joinErrs(errs []string) string {
	result := ""
	for i, e := range errs {
		if i > 0 {
			result += "; "
		}
		result += e
	}
	return result
}
```

- [x] **Step 4: Run tests to confirm they pass**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/... -run TestRouter -v
```

Expected: PASS all four router tests.

- [x] **Step 5: Run all provider tests**

```bash
cd /Users/joe/dev/devkit && go test ./internal/providers/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/joe/dev/devkit && git add internal/providers/ && git commit -m "feat(providers): add Router with tier routing, role mapping, and fallback chain"
```

---

## Task 7: Update `internal/loop` with generic `RunAgentLoop`

**Files:**

- Modify: `internal/loop/loop.go`
- Modify: `internal/loop/loop_test.go`

- [x] **Step 1: Write the failing test**

Add to `internal/loop/loop_test.go`:

```go
package loop_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/loop"
	"github.com/89jobrien/devkit/internal/providers"
	"github.com/89jobrien/devkit/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAgentProvider struct{ response string }

func (s stubAgentProvider) Chat(_ context.Context, _ string) (string, error) {
	return s.response, nil
}
func (s stubAgentProvider) RunAgent(_ context.Context, _ string, _ []tools.Tool) (string, error) {
	return s.response, nil
}

func TestRunAgentLoop_UsesProvider(t *testing.T) {
	p := stubAgentProvider{response: "result from provider"}
	result, err := loop.RunAgentLoop(context.Background(), p, "do work", nil)
	require.NoError(t, err)
	assert.Equal(t, "result from provider", result)
}

// Verify stubAgentProvider satisfies the providers.AgentProvider interface.
var _ providers.AgentProvider = stubAgentProvider{}
```

- [x] **Step 2: Run to confirm it fails**

```bash
cd /Users/joe/dev/devkit && go test ./internal/loop/... -run TestRunAgentLoop
```

Expected: compile error — `loop.RunAgentLoop` undefined.

- [x] **Step 3: Add `RunAgentLoop` to `loop.go`**

Add at the end of `internal/loop/loop.go` (keep existing `RunAgent` intact):

```go
// RunAgentLoop runs a tool-use agent loop using any AgentProvider.
// This is the provider-agnostic counterpart to RunAgent (which is Anthropic-SDK-specific).
func RunAgentLoop(ctx context.Context, p interface {
	RunAgent(ctx context.Context, prompt string, ts []tools.Tool) (string, error)
}, prompt string, ts []tools.Tool) (string, error) {
	return p.RunAgent(ctx, prompt, ts)
}
```

Also add the import for `tools` if not already present:

```go
"github.com/89jobrien/devkit/internal/tools"
```

- [x] **Step 4: Run tests**

```bash
cd /Users/joe/dev/devkit && go test ./internal/loop/... -v
```

Expected: all PASS (existing tests + new test).

- [ ] **Step 5: Commit**

```bash
cd /Users/joe/dev/devkit && git add internal/loop/ && git commit -m "feat(loop): add RunAgentLoop for provider-agnostic tool-use"
```

---

## Task 8: Wire `.devkit.toml` provider overrides into Config

**Files:**

- Modify: `cmd/devkit/config.go`

- [x] **Step 1: Write the failing test**

Add to `cmd/devkit/` a new file `config_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigProviderOverrides(t *testing.T) {
	dir := t.TempDir()
	toml := `
[providers]
primary = "openai"
coding_model = "gpt-5.4-custom"
fast_model = "gemini-3-flash-preview"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".devkit.toml"), []byte(toml), 0o644))
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(orig)

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.Providers.Primary)
	assert.Equal(t, "gpt-5.4-custom", cfg.Providers.CodingModel)
	assert.Equal(t, "gemini-3-flash-preview", cfg.Providers.FastModel)
}
```

- [x] **Step 2: Run to confirm it fails**

```bash
cd /Users/joe/dev/devkit && go test ./cmd/devkit/... -run TestLoadConfigProviderOverrides
```

Expected: compile error — `cfg.Providers` undefined.

- [x] **Step 3: Add `Providers` section to `Config`**

In `cmd/devkit/config.go`, add after the `Diagnose` struct field:

```go
	Providers struct {
		Primary           string `toml:"primary"`
		FastModel         string `toml:"fast_model"`
		BalancedModel     string `toml:"balanced_model"`
		LargeContextModel string `toml:"large_context_model"`
		CodingModel       string `toml:"coding_model"`
	} `toml:"providers"`
```

- [x] **Step 4: Run tests**

```bash
cd /Users/joe/dev/devkit && go test ./cmd/devkit/... -run TestLoadConfigProviderOverrides -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/joe/dev/devkit && git add cmd/devkit/config.go cmd/devkit/config_test.go && git commit -m "feat(config): add [providers] section for model/provider overrides"
```

---

## Task 9: Replace `cmd/devkit/runner.go` with Router-backed wiring

**Files:**

- Replace: `cmd/devkit/runner.go`

- [x] **Step 1: Write the failing test**

Add to `cmd/devkit/` a new file `runner_test.go`:

```go
package main

import (
	"testing"

	"github.com/89jobrien/devkit/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRouterFromConfig_NoKeys(t *testing.T) {
	cfg := &Config{}
	r, err := newRouterFromConfig(cfg)
	require.NoError(t, err)
	assert.NotNil(t, r)
}

func TestNewRouterFromConfig_WithOverrides(t *testing.T) {
	cfg := &Config{}
	cfg.Providers.Primary = "openai"
	cfg.Providers.CodingModel = "gpt-5.4-custom"
	r, err := newRouterFromConfig(cfg)
	require.NoError(t, err)
	assert.NotNil(t, r)
	_ = r.For(providers.TierCoding) // must not panic
}
```

- [x] **Step 2: Run to confirm it fails**

```bash
cd /Users/joe/dev/devkit && go test ./cmd/devkit/... -run TestNewRouter
```

Expected: compile error — `newRouterFromConfig` undefined.

- [x] **Step 3: Rewrite `cmd/devkit/runner.go`**

Replace the entire file:

```go
// cmd/devkit/runner.go
package main

import (
	"os"

	"github.com/89jobrien/devkit/internal/providers"
)

// newRouterFromConfig constructs a Router using API keys from the environment
// and any model overrides from .devkit.toml.
func newRouterFromConfig(cfg *Config) (*providers.Router, error) {
	return providers.NewRouter(providers.RouterConfig{
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIKey:    os.Getenv("OPENAI_API_KEY"),
		GeminiKey:    os.Getenv("GEMINI_API_KEY"),
		Overrides: providers.TierOverrides{
			PrimaryProvider:   cfg.Providers.Primary,
			FastModel:         cfg.Providers.FastModel,
			BalancedModel:     cfg.Providers.BalancedModel,
			LargeContextModel: cfg.Providers.LargeContextModel,
			CodingModel:       cfg.Providers.CodingModel,
		},
	}), nil
}
```

Note: The old `agentRunner`, `openAIRunner`, and `bearerTransport` types are removed. `main.go` will be updated in the next task to use `newRouterFromConfig`.

- [x] **Step 4: Run tests**

```bash
cd /Users/joe/dev/devkit && go test ./cmd/devkit/... -run TestNewRouter -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/joe/dev/devkit && git add cmd/devkit/runner.go cmd/devkit/runner_test.go && git commit -m "feat(runner): replace agentRunner/openAIRunner with Router-backed wiring"
```

---

## Task 10: Update `cmd/devkit/main.go` to use Router for all commands

**Files:**

- Modify: `cmd/devkit/main.go`

This task replaces all `newAgentRunner()` and `newOpenAIRunner()` call sites with `router.AgentRunnerFor(providers.TierCoding, ...)` or `router.For(tier)`, and maps council roles to tiers.

- [x] **Step 1: Update `councilCmd` in `main.go`**

Replace the council runner wiring (lines ~138-158 in the original) with:

```go
cfg, err := LoadConfig()
if err != nil {
    return err
}
if cfg.Project.Name != "" {
    os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
}

diff := gitDiff(councilBase)
commits := gitLog(councilBase)
router, err := newRouterFromConfig(cfg)
if err != nil {
    return err
}
sha := devlog.GitShortSHA()
id := devlog.Start("council", map[string]string{"base": councilBase, "mode": councilMode})
start := time.Now()

// Build per-role runners based on semantic tier routing.
roleRunners := make(map[string]council.Runner)
for _, role := range []string{"creative-explorer", "performance-analyst", "general-analyst", "security-reviewer", "strict-critic"} {
    tier := providers.TierForRole(role)
    roleRunners[role] = router.For(tier)
}

councilCfg := council.Config{
    Base:    councilBase,
    Mode:    councilMode,
    Diff:    diff,
    Commits: commits,
    Runner:  router.For(providers.TierBalanced), // default for unrecognized roles
    Runners: roleRunners,
}
```

Also add `"github.com/89jobrien/devkit/internal/providers"` to imports.

- [x] **Step 2: Update `reviewCmd` in `main.go`**

Replace `runner := newAgentRunner()` with:

```go
cfg, err := LoadConfig()
if err != nil {
    return err
}
router, err := newRouterFromConfig(cfg)
if err != nil {
    return err
}
```

Replace the `review.Config` runner with:

```go
Runner: review.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
    wd, _ := os.Getwd()
    agentTools := []tools.Tool{
        tools.ReadTool(wd),
        tools.GlobTool(wd),
        tools.GrepTool(wd),
    }
    return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
}),
```

Add `"github.com/89jobrien/devkit/internal/tools"` to imports if not present.

- [x] **Step 3: Update `metaCmd` in `main.go`**

Replace `runner := newAgentRunner()` with:

```go
cfg, _ := LoadConfig()
if cfg.Project.Name != "" {
    os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
}
router, err := newRouterFromConfig(cfg)
if err != nil {
    return err
}
```

Replace the `meta.Run` runner with:

```go
meta.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
    wd, _ := os.Getwd()
    agentTools := []tools.Tool{
        tools.ReadTool(wd),
        tools.GlobTool(wd),
        tools.GrepTool(wd),
        tools.BashTool(30_000, nil),
    }
    return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
}),
```

- [x] **Step 4: Update `diagnoseCmd` in `main.go`**

Replace `runner := newAgentRunner()` and its `confirmFn` setup with:

```go
cfg, err := LoadConfig()
if err != nil {
    return err
}
if cfg.Project.Name != "" {
    os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
}
router, err := newRouterFromConfig(cfg)
if err != nil {
    return err
}
var confirmFn func(string) bool
if diagnoseConfirm {
    scanner := bufio.NewScanner(os.Stdin)
    confirmFn = func(c string) bool {
        fmt.Fprintf(os.Stderr, "\nBashTool wants to run: %s\nAllow? [y/N] ", c)
        if !scanner.Scan() {
            return false
        }
        return strings.TrimSpace(strings.ToLower(scanner.Text())) == "y"
    }
}
```

Replace the `diagnose.Config` runner with:

```go
Runner: diagnose.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
    wd, _ := os.Getwd()
    agentTools := []tools.Tool{
        tools.ReadTool(wd),
        tools.GlobTool(wd),
        tools.GrepTool(wd),
        tools.BashTool(30_000, confirmFn),
    }
    return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
}),
```

- [x] **Step 5: Build to verify compilation**

```bash
cd /Users/joe/dev/devkit && go build ./cmd/devkit ./cmd/ci-agent
```

Expected: no errors.

- [x] **Step 6: Run all tests**

```bash
cd /Users/joe/dev/devkit && go test ./...
```

Expected: all 37+ tests PASS.

- [ ] **Step 7: Commit**

```bash
cd /Users/joe/dev/devkit && git add cmd/devkit/main.go && git commit -m "feat(main): wire all commands through Router with semantic tier routing"
```

---

## Task 11: Final verification and cleanup

- [x] **Step 1: Run full test suite**

```bash
cd /Users/joe/dev/devkit && go test ./... -v 2>&1 | tail -30
```

Expected: all tests PASS, no failures.

- [x] **Step 2: Run go vet**

```bash
cd /Users/joe/dev/devkit && go vet ./...
```

Expected: no output (no issues).

- [x] **Step 3: Build both binaries**

```bash
cd /Users/joe/dev/devkit && go build ./cmd/devkit ./cmd/ci-agent
```

Expected: success.

- [x] **Step 4: Verify `internal/loop/loop.go` still exports `RunAgent` (no regressions)**

```bash
cd /Users/joe/dev/devkit && grep -n "^func RunAgent" internal/loop/loop.go
```

Expected: line with `func RunAgent(` present.

- [ ] **Step 5: Commit final state**

```bash
cd /Users/joe/dev/devkit && git add -A && git commit -m "chore: final verification — provider fallback & routing complete"
```
