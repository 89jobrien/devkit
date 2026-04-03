# devkit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `github.com/89jobrien/devkit` — a Go CLI toolkit providing self-correcting CI diagnosis, multi-role council review, diff review, and a parallel meta-agent, installable into any project via `install.sh`.

**Architecture:** Compiled binary (`devkit`) with cobra subcommands backed by shared internal packages. Standalone `cmd/ci-agent` invoked via `go run` in CI pipelines. Custom tool-use loop implements Read/Glob/Grep as Anthropic function-calling tools.

**Tech Stack:** Go 1.23+, `github.com/anthropics/anthropic-sdk-go`, `github.com/spf13/cobra`, `github.com/BurntSushi/toml`, `golang.org/x/sync/errgroup`, `github.com/stretchr/testify`

---

## File Map

| File                                 | Responsibility                                         |
| ------------------------------------ | ------------------------------------------------------ |
| `go.mod`                             | Module declaration, dependency pinning                 |
| `VERSION`                            | Semver string read by upgrade logic                    |
| `Justfile`                           | Build/test/install recipes for devkit itself           |
| `install.sh`                         | Generate `.devkit.toml` + CI YAMLs in target project   |
| `upgrade.sh`                         | `go install @latest` + CI template regeneration        |
| `ci/gitea.yml`                       | Gitea Actions CI template                              |
| `ci/github.yml`                      | GitHub Actions CI template                             |
| `internal/log/log.go`                | JSONL telemetry + per-commit markdown sinks            |
| `internal/log/log_test.go`           | Unit tests for all log functions                       |
| `internal/tools/tools.go`            | Read/Glob/Grep as `Tool` structs with handlers         |
| `internal/tools/tools_test.go`       | Unit tests with temp dirs                              |
| `internal/loop/loop.go`              | Tool-use execution loop + `Tool` type                  |
| `internal/loop/loop_test.go`         | Tests against mock Anthropic HTTP server               |
| `internal/platform/platform.go`      | `Platform` interface + `JobLog` type + `New()` factory |
| `internal/platform/gitea.go`         | Gitea API implementation                               |
| `internal/platform/github.go`        | GitHub API implementation                              |
| `internal/platform/platform_test.go` | Tests against httptest mock servers                    |
| `internal/council/council.go`        | Role definitions, concurrent execution, synthesis      |
| `internal/council/council_test.go`   | Unit tests with stub RunAgent                          |
| `internal/review/review.go`          | Single-agent diff review                               |
| `internal/review/review_test.go`     | Unit tests with stub RunAgent                          |
| `internal/meta/meta.go`              | Designer → parallel workers → synthesis                |
| `internal/meta/meta_test.go`         | Unit tests with stub designer output                   |
| `cmd/ci-agent/main.go`               | Standalone CI agent (no Anthropic SDK dep)             |
| `cmd/ci-agent/providers.go`          | Raw HTTP LLM fallback chain                            |
| `cmd/ci-agent/main_test.go`          | Unit tests for provider selection + env parsing        |
| `cmd/devkit/main.go`                 | cobra root + subcommand wiring                         |

---

## Task 1: Module scaffold

**Files:**

- Create: `go.mod`
- Create: `go.sum` (generated)
- Create: `VERSION`
- Create: `Justfile`
- Create: `.gitignore`

- [x] **Step 1: Initialize Go module**

```bash
cd /Users/joe/dev/devkit
go mod init github.com/89jobrien/devkit
```

- [x] **Step 2: Add dependencies**

```bash
go get github.com/anthropics/anthropic-sdk-go@latest
go get github.com/spf13/cobra@latest
go get github.com/BurntSushi/toml@latest
go get golang.org/x/sync@latest
go get github.com/stretchr/testify@latest
```

- [x] **Step 3: Create VERSION**

```
1.0.0
```

- [x] **Step 4: Create .gitignore**

```
devkit
dist/
```

- [x] **Step 5: Create Justfile**

```makefile
build:
    go build -o devkit ./cmd/devkit

install:
    go install ./cmd/devkit

test:
    go test ./...

lint:
    go vet ./...
```

- [x] **Step 6: Verify build scaffolding compiles**

Create `cmd/devkit/main.go` with just `package main\nfunc main() {}` and `cmd/ci-agent/main.go` the same, then:

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum VERSION Justfile .gitignore cmd/
git commit -m "feat: initialize Go module scaffold"
```

---

## Task 2: `internal/log` — telemetry

**Files:**

- Create: `internal/log/log.go`
- Create: `internal/log/log_test.go`

- [x] **Step 1: Write failing tests**

```go
// internal/log/log_test.go
package log_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartComplete(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DEVKIT_LOG_DIR", dir)
	t.Setenv("DEVKIT_PROJECT", "testproj")

	id := log.Start("council", map[string]string{"base": "main"})
	assert.NotEmpty(t, id)

	log.Complete(id, "council", map[string]string{"base": "main"}, "output text", 1500*time.Millisecond)

	data, err := os.ReadFile(filepath.Join(dir, "testproj", "agent-runs.jsonl"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Len(t, lines, 2)

	var start map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &start))
	assert.Equal(t, "council", start["command"])
	assert.Equal(t, "running", start["status"])

	var complete map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &complete))
	assert.Equal(t, "complete", complete["status"])
	assert.Equal(t, "output text", complete["output"])
}

func TestSaveCommitLog(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DEVKIT_LOG_DIR", dir)
	t.Setenv("DEVKIT_PROJECT", "testproj")

	path, err := log.SaveCommitLog("abc1234", "council", "## Results\nfoo", map[string]string{"mode": "core"})
	require.NoError(t, err)
	assert.Contains(t, path, "abc1234-council.md")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "## Results")
	assert.Contains(t, string(data), "mode: core")
}

func TestProjectNameFallback(t *testing.T) {
	t.Setenv("DEVKIT_PROJECT", "")
	// Should not panic; falls back to git or "unknown"
	name := log.ProjectName()
	assert.NotEmpty(t, name)
}
```

- [x] **Step 2: Run — verify FAIL**

```bash
go test ./internal/log/... 2>&1 | head -5
```

Expected: compile error (package doesn't exist yet).

- [x] **Step 3: Implement `internal/log/log.go`**

```go
// internal/log/log.go
package log

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunID is an ISO-8601 timestamp used to correlate start/complete entries.
type RunID string

// ProjectName returns the project name for log namespacing.
// Priority: DEVKIT_PROJECT env → nearest .devkit.toml name → git repo basename → "unknown".
func ProjectName() string {
	if v := os.Getenv("DEVKIT_PROJECT"); v != "" {
		return v
	}
	// Try git basename
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		return filepath.Base(strings.TrimSpace(string(out)))
	}
	return "unknown"
}

// GitShortSHA returns the short git SHA of HEAD, or "unknown".
func GitShortSHA() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func logDir() string {
	if v := os.Getenv("DEVKIT_LOG_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".dev-agents")
}

func projectDir() string {
	return filepath.Join(logDir(), ProjectName())
}

func jsonlPath() string {
	return filepath.Join(projectDir(), "agent-runs.jsonl")
}

func appendJSONL(record map[string]any) {
	path := jsonlPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(record)
	_, _ = f.Write(append(b, '\n'))
}

// Start records a run-start entry and returns a RunID.
func Start(command string, args map[string]string) RunID {
	id := RunID(time.Now().UTC().Format(time.RFC3339Nano))
	appendJSONL(map[string]any{
		"run_id":  string(id),
		"command": command,
		"args":    args,
		"status":  "running",
	})
	return id
}

// Complete records a run-completion entry.
func Complete(id RunID, command string, args map[string]string, output string, duration time.Duration) {
	appendJSONL(map[string]any{
		"run_id":      string(id),
		"command":     command,
		"args":        args,
		"status":      "complete",
		"duration_ms": duration.Milliseconds(),
		"output":      output,
	})
}

// SaveCommitLog writes ~/.dev-agents/<project>/ai-logs/<sha>-<command>.md and returns the path.
func SaveCommitLog(sha, command, content string, meta map[string]string) (string, error) {
	dir := filepath.Join(projectDir(), "ai-logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.md", sha, command))

	var header strings.Builder
	header.WriteString(fmt.Sprintf("# %s · %s\n\n", command, sha))
	for k, v := range meta {
		header.WriteString(fmt.Sprintf("- **%s**: %s\n", k, v))
	}
	header.WriteString("\n---\n\n")

	if err := os.WriteFile(path, []byte(header.String()+content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
```

- [x] **Step 4: Run tests — verify PASS**

```bash
go test ./internal/log/... -v
```

Expected: all 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/log/
git commit -m "feat: add internal/log telemetry package"
```

---

## Task 3: `internal/tools` — Read/Glob/Grep

**Files:**

- Create: `internal/tools/tools.go`
- Create: `internal/tools/tools_test.go`

- [x] **Step 1: Write failing tests**

```go
// internal/tools/tools_test.go
package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/89jobrien/devkit/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadTool(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0o644))

	tool := tools.ReadTool(dir)
	input, _ := json.Marshal(map[string]string{"path": "hello.txt"})
	result, err := tool.Handler(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "hello world", result)
}

func TestReadToolRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	tool := tools.ReadTool(dir)
	input, _ := json.Marshal(map[string]string{"path": "../secret"})
	_, err := tool.Handler(context.Background(), input)
	assert.ErrorContains(t, err, "outside")
}

func TestGlobTool(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte(""), 0o644))

	tool := tools.GlobTool(dir)
	input, _ := json.Marshal(map[string]string{"pattern": "*.go"})
	result, err := tool.Handler(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result, "a.go")
	assert.Contains(t, result, "b.go")
	assert.NotContains(t, result, "c.txt")
}

func TestGrepTool(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644))

	tool := tools.GrepTool(dir)
	input, _ := json.Marshal(map[string]string{"pattern": "func main", "glob": "*.go"})
	result, err := tool.Handler(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result, "main.go:2")
	assert.Contains(t, result, "func main")
}
```

- [x] **Step 2: Run — verify FAIL**

```bash
go test ./internal/tools/... 2>&1 | head -5
```

- [x] **Step 3: Implement `internal/tools/tools.go`**

```go
// internal/tools/tools.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// Tool pairs an Anthropic tool definition with its handler function.
type Tool struct {
	Definition anthropic.ToolUnionParam
	// Handler receives raw JSON input and returns a string result or error.
	Handler func(ctx context.Context, input json.RawMessage) (string, error)
}

func validatePath(root, rel string) (string, error) {
	// Resolve relative to root and ensure result stays within root.
	abs := filepath.Join(root, rel)
	clean, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	rootClean, _ := filepath.Abs(root)
	if !strings.HasPrefix(clean, rootClean+string(os.PathSeparator)) && clean != rootClean {
		return "", fmt.Errorf("path %q is outside working directory", rel)
	}
	return clean, nil
}

// ReadTool returns a Tool that reads a file relative to root.
func ReadTool(root string) Tool {
	return Tool{
		Definition: anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        "Read",
			Description: anthropic.String("Read a file and return its contents."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]interface{}{
					"path": map[string]string{"type": "string", "description": "File path relative to working directory"},
				},
			},
		}},
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", err
			}
			abs, err := validatePath(root, args.Path)
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	}
}

// GlobTool returns a Tool that matches a glob pattern relative to root.
func GlobTool(root string) Tool {
	return Tool{
		Definition: anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        "Glob",
			Description: anthropic.String("Match files against a glob pattern. Returns newline-separated paths."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]interface{}{
					"pattern": map[string]string{"type": "string", "description": "Glob pattern"},
				},
			},
		}},
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Pattern string `json:"pattern"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", err
			}
			matches, err := filepath.Glob(filepath.Join(root, args.Pattern))
			if err != nil {
				return "", err
			}
			// Make paths relative to root for cleaner output
			rel := make([]string, 0, len(matches))
			for _, m := range matches {
				r, _ := filepath.Rel(root, m)
				rel = append(rel, r)
			}
			return strings.Join(rel, "\n"), nil
		},
	}
}

// GrepTool returns a Tool that searches file content for a regex pattern.
func GrepTool(root string) Tool {
	return Tool{
		Definition: anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        "Grep",
			Description: anthropic.String("Search file content for a regex pattern. Returns file:line matches."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]interface{}{
					"pattern": map[string]string{"type": "string", "description": "Regular expression pattern"},
					"glob":    map[string]string{"type": "string", "description": "Glob pattern to filter files (e.g. '*.go')"},
				},
			},
		}},
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Pattern string `json:"pattern"`
				Glob    string `json:"glob"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", err
			}
			re, err := regexp.Compile(args.Pattern)
			if err != nil {
				return "", fmt.Errorf("invalid pattern: %w", err)
			}
			glob := args.Glob
			if glob == "" {
				glob = "*"
			}
			matches, _ := filepath.Glob(filepath.Join(root, glob))

			var results []string
			for _, path := range matches {
				data, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				rel, _ := filepath.Rel(root, path)
				for i, line := range strings.Split(string(data), "\n") {
					if re.MatchString(line) {
						results = append(results, fmt.Sprintf("%s:%d: %s", rel, i+1, line))
					}
				}
			}
			return strings.Join(results, "\n"), nil
		},
	}
}

// Definitions returns the Anthropic tool definitions for all three tools (for use in API calls).
func Definitions(ts []Tool) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, len(ts))
	for i, t := range ts {
		out[i] = t.Definition
	}
	return out
}
```

- [x] **Step 4: Run tests — verify PASS**

```bash
go test ./internal/tools/... -v
```

Expected: 4 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/
git commit -m "feat: add internal/tools Read/Glob/Grep"
```

---

## Task 4: `internal/loop` — tool-use execution loop

**Files:**

- Create: `internal/loop/loop.go`
- Create: `internal/loop/loop_test.go`

- [x] **Step 1: Write failing tests**

```go
// internal/loop/loop_test.go
package loop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/89jobrien/devkit/internal/loop"
	"github.com/89jobrien/devkit/internal/tools"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockResponse builds a minimal Anthropic messages response JSON.
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
			// Return a tool_use block
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
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct{ Text string `json:"text"` }
			json.Unmarshal(input, &args)
			return fmt.Sprintf("pong: %s", args.Text), nil
		},
	}

	result, err := loop.RunAgent(context.Background(), client, "use the echo tool", []tools.Tool{echoTool})
	require.NoError(t, err)
	assert.Equal(t, "done", result)
	assert.Equal(t, 2, calls)
}
```

- [x] **Step 2: Run — verify FAIL**

```bash
go test ./internal/loop/... 2>&1 | head -5
```

- [x] **Step 3: Implement `internal/loop/loop.go`**

```go
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

// RunAgent runs a tool-use loop until stop_reason is "end_turn".
// Returns the concatenated text content of the final response.
func RunAgent(ctx context.Context, client *anthropic.Client, prompt string, ts []tools.Tool) (string, error) {
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

		// Handle tool_use blocks
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
			result, err := t.Handler(ctx, json.RawMessage(tu.JSON.Input.Raw()))
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

- [x] **Step 4: Run tests — verify PASS**

```bash
go test ./internal/loop/... -v
```

Expected: 2 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/loop/
git commit -m "feat: add internal/loop tool-use execution loop"
```

---

## Task 5: `internal/platform` — Gitea + GitHub API clients

**Files:**

- Create: `internal/platform/platform.go`
- Create: `internal/platform/gitea.go`
- Create: `internal/platform/github.go`
- Create: `internal/platform/platform_test.go`

- [x] **Step 1: Write failing tests**

```go
// internal/platform/platform_test.go
package platform_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/89jobrien/devkit/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGiteaFetchFailedJobLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/owner/repo/actions/runs/42/jobs":
			json.NewEncoder(w).Encode(map[string]any{
				"jobs": []map[string]any{
					{"id": 1, "name": "lint", "conclusion": "failure"},
					{"id": 2, "name": "test", "conclusion": "success"},
				},
			})
		case "/api/v1/repos/owner/repo/actions/jobs/1/logs":
			w.Write([]byte("error: undefined variable"))
		}
	}))
	defer srv.Close()

	p, err := platform.New("gitea", "owner/repo", "42", "abc123", "tok", srv.URL)
	require.NoError(t, err)

	logs, err := p.FetchFailedJobLogs(context.Background(), "42")
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "lint", logs[0].Name)
	assert.Contains(t, logs[0].Log, "error: undefined variable")
}

func TestGiteaCreateIssue(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/owner/repo/labels":
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode([]map[string]any{})
			} else {
				json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "ci-failure"})
			}
		case "/api/v1/repos/owner/repo/issues":
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode([]map[string]any{})
			} else {
				json.NewDecoder(r.Body).Decode(&capturedBody)
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]any{"number": 7})
			}
		case "/api/v1/repos/owner/repo/statuses/abc123":
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer srv.Close()

	p, err := platform.New("gitea", "owner/repo", "42", "abc123", "tok", srv.URL)
	require.NoError(t, err)

	require.NoError(t, p.EnsureLabelExists(context.Background()))
	num, err := p.CreateIssue(context.Background(), "abc123", "root cause: X", "anthropic", []string{"lint"}, "42")
	require.NoError(t, err)
	assert.Equal(t, 7, num)
	assert.Contains(t, capturedBody["body"], "<!-- sha: abc123 -->")
}

func TestGitHubFetchFailedJobLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/actions/runs/42/jobs":
			json.NewEncoder(w).Encode(map[string]any{
				"jobs": []map[string]any{
					{"id": 10, "name": "build", "conclusion": "failure"},
				},
			})
		case "/repos/owner/repo/actions/jobs/10/logs":
			// GitHub returns a redirect; simulate direct log return
			w.Write([]byte("FAIL: compilation error"))
		}
	}))
	defer srv.Close()

	p, err := platform.New("github", "owner/repo", "42", "abc123", "ghp_token", srv.URL)
	require.NoError(t, err)

	logs, err := p.FetchFailedJobLogs(context.Background(), "42")
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "build", logs[0].Name)
}
```

- [x] **Step 2: Run — verify FAIL**

```bash
go test ./internal/platform/... 2>&1 | head -5
```

- [x] **Step 3: Implement `internal/platform/platform.go`**

```go
// internal/platform/platform.go
package platform

import (
	"context"
	"fmt"
)

// JobLog holds the name and log text of a failed CI job.
type JobLog struct {
	Name string
	Log  string
}

const maxLogBytes = 30_000

// Platform abstracts Gitea and GitHub CI operations.
type Platform interface {
	SetCommitStatus(ctx context.Context, state, description string) error
	EnsureLabelExists(ctx context.Context) error
	FindIssueForCommit(ctx context.Context, sha string) (int, bool, error)
	CreateIssue(ctx context.Context, sha, diagnosis, provider string, failedJobs []string, runID string) (int, error)
	AddComment(ctx context.Context, issueNumber int, diagnosis, provider string) error
	FetchFailedJobLogs(ctx context.Context, runID string) ([]JobLog, error)
}

// New returns the appropriate Platform implementation based on name ("gitea" or "github").
// baseURL is optional and used for testing (overrides default API base).
func New(name, repo, runID, commitSHA, token, baseURL string) (Platform, error) {
	switch name {
	case "gitea":
		if baseURL == "" {
			return nil, fmt.Errorf("gitea requires GITEA_URL")
		}
		return &giteaPlatform{repo: repo, runID: runID, sha: commitSHA, token: token, baseURL: baseURL}, nil
	case "github":
		base := baseURL
		if base == "" {
			base = "https://api.github.com"
		}
		return &githubPlatform{repo: repo, runID: runID, sha: commitSHA, token: token, baseURL: base}, nil
	default:
		return nil, fmt.Errorf("unknown platform: %q (want gitea or github)", name)
	}
}

func truncateLast(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "...[truncated]...\n" + s[len(s)-n:]
}
```

- [x] **Step 4: Implement `internal/platform/gitea.go`**

```go
// internal/platform/gitea.go
package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type giteaPlatform struct {
	repo, runID, sha, token, baseURL string
}

func (g *giteaPlatform) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.baseURL+"/api/v1"+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+g.token)
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func (g *giteaPlatform) FetchFailedJobLogs(ctx context.Context, runID string) ([]JobLog, error) {
	resp, err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/actions/runs/%s/jobs?limit=50", g.repo, runID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Jobs []struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			Conclusion string `json:"conclusion"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var logs []JobLog
	for _, j := range data.Jobs {
		if j.Conclusion != "failure" {
			continue
		}
		lr, err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/actions/jobs/%d/logs", g.repo, j.ID), nil)
		if err != nil {
			logs = append(logs, JobLog{Name: j.Name, Log: fmt.Sprintf("(log unavailable: %v)", err)})
			continue
		}
		defer lr.Body.Close()
		raw, _ := io.ReadAll(lr.Body)
		logs = append(logs, JobLog{Name: j.Name, Log: truncateLast(string(raw), maxLogBytes)})
	}
	return logs, nil
}

func (g *giteaPlatform) SetCommitStatus(ctx context.Context, state, description string) error {
	if len(description) > 140 {
		description = description[:140]
	}
	resp, err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/statuses/%s", g.repo, g.sha), map[string]string{
		"context":     "ci/agent-diagnosis",
		"state":       state,
		"description": description,
	})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (g *giteaPlatform) EnsureLabelExists(ctx context.Context) error {
	resp, err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/labels", g.repo), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var labels []struct{ Name string `json:"name"` }
	json.NewDecoder(resp.Body).Decode(&labels)
	for _, l := range labels {
		if l.Name == "ci-failure" {
			return nil
		}
	}
	cr, err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/labels", g.repo), map[string]string{
		"name": "ci-failure", "color": "#e11d48",
	})
	if err != nil {
		return err
	}
	cr.Body.Close()
	return nil
}

func (g *giteaPlatform) FindIssueForCommit(ctx context.Context, sha string) (int, bool, error) {
	marker := fmt.Sprintf("<!-- sha: %s -->", sha)
	for page := 1; ; page++ {
		resp, err := g.do(ctx, http.MethodGet,
			fmt.Sprintf("/repos/%s/issues?state=open&type=issues&labels=ci-failure&limit=50&page=%d", g.repo, page), nil)
		if err != nil {
			return 0, false, err
		}
		defer resp.Body.Close()
		var issues []struct {
			Number int    `json:"number"`
			Body   string `json:"body"`
		}
		json.NewDecoder(resp.Body).Decode(&issues)
		if len(issues) == 0 {
			return 0, false, nil
		}
		for _, i := range issues {
			if strings.Contains(i.Body, marker) {
				return i.Number, true, nil
			}
		}
		// page incremented by for loop header — no page++ here
	}
}

func (g *giteaPlatform) CreateIssue(ctx context.Context, sha, diagnosis, provider string, failedJobs []string, runID string) (int, error) {
	title := fmt.Sprintf("CI failure: %s — %s", sha[:8], strings.Join(failedJobs, ", "))
	runURL := fmt.Sprintf("%s/%s/actions/runs/%s", g.baseURL, g.repo, runID)
	body := fmt.Sprintf("## CI Failure Diagnosis\n\n**Jobs:** %s\n**Provider:** %s\n**Commit:** %s\n**Run:** [%s](%s)\n\n%s\n\n---\n*Diagnosed by ci-agent.*\n<!-- sha: %s -->",
		strings.Join(failedJobs, ", "), provider, sha, runID, runURL, diagnosis, sha)

	resp, err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues", g.repo), map[string]any{
		"title": title, "body": body, "labels": []string{"ci-failure"},
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var result struct{ Number int `json:"number"` }
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Number, nil
}

func (g *giteaPlatform) AddComment(ctx context.Context, issueNumber int, diagnosis, provider string) error {
	body := fmt.Sprintf("## Re-run Diagnosis\n\n**Provider:** %s\n\n%s", provider, diagnosis)
	resp, err := g.do(ctx, http.MethodPost,
		fmt.Sprintf("/repos/%s/issues/%d/comments", g.repo, issueNumber),
		map[string]string{"body": body})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
```

- [x] **Step 5: Implement `internal/platform/github.go`**

```go
// internal/platform/github.go
package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type githubPlatform struct {
	repo, runID, sha, token, baseURL string
}

func (g *githubPlatform) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	return http.DefaultClient.Do(req)
}

func (g *githubPlatform) FetchFailedJobLogs(ctx context.Context, runID string) ([]JobLog, error) {
	resp, err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/actions/runs/%s/jobs", g.repo, runID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Jobs []struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			Conclusion string `json:"conclusion"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var logs []JobLog
	for _, j := range data.Jobs {
		if j.Conclusion != "failure" {
			continue
		}
		lr, err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/actions/jobs/%d/logs", g.repo, j.ID), nil)
		if err != nil {
			logs = append(logs, JobLog{Name: j.Name, Log: fmt.Sprintf("(log unavailable: %v)", err)})
			continue
		}
		defer lr.Body.Close()
		raw, _ := io.ReadAll(lr.Body)
		logs = append(logs, JobLog{Name: j.Name, Log: truncateLast(string(raw), maxLogBytes)})
	}
	return logs, nil
}

func (g *githubPlatform) SetCommitStatus(ctx context.Context, state, description string) error {
	if len(description) > 140 {
		description = description[:140]
	}
	resp, err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/statuses/%s", g.repo, g.sha), map[string]string{
		"context":     "ci/agent-diagnosis",
		"state":       state,
		"description": description,
	})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (g *githubPlatform) EnsureLabelExists(ctx context.Context) error {
	resp, err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/labels", g.repo), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var labels []struct{ Name string `json:"name"` }
	json.NewDecoder(resp.Body).Decode(&labels)
	for _, l := range labels {
		if l.Name == "ci-failure" {
			return nil
		}
	}
	cr, err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/labels", g.repo),
		map[string]string{"name": "ci-failure", "color": "#e11d48"})
	if err != nil {
		return err
	}
	cr.Body.Close()
	return nil
}

func (g *githubPlatform) FindIssueForCommit(ctx context.Context, sha string) (int, bool, error) {
	marker := fmt.Sprintf("<!-- sha: %s -->", sha)
	for page := 1; ; page++ {
		resp, err := g.do(ctx, http.MethodGet,
			fmt.Sprintf("/repos/%s/issues?state=open&labels=ci-failure&per_page=50&page=%d", g.repo, page), nil)
		if err != nil {
			return 0, false, err
		}
		defer resp.Body.Close()
		var issues []struct {
			Number int    `json:"number"`
			Body   string `json:"body"`
		}
		json.NewDecoder(resp.Body).Decode(&issues)
		if len(issues) == 0 {
			return 0, false, nil
		}
		for _, i := range issues {
			if strings.Contains(i.Body, marker) {
				return i.Number, true, nil
			}
		}
		// page incremented by for loop header — no page++ here
	}
}

func (g *githubPlatform) CreateIssue(ctx context.Context, sha, diagnosis, provider string, failedJobs []string, runID string) (int, error) {
	title := fmt.Sprintf("CI failure: %s — %s", sha[:8], strings.Join(failedJobs, ", "))
	runURL := fmt.Sprintf("https://github.com/%s/actions/runs/%s", g.repo, runID)
	body := fmt.Sprintf("## CI Failure Diagnosis\n\n**Jobs:** %s\n**Provider:** %s\n**Commit:** %s\n**Run:** [%s](%s)\n\n%s\n\n---\n*Diagnosed by ci-agent.*\n<!-- sha: %s -->",
		strings.Join(failedJobs, ", "), provider, sha, runID, runURL, diagnosis, sha)

	resp, err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues", g.repo), map[string]any{
		"title": title, "body": body, "labels": []string{"ci-failure"},
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var result struct{ Number int `json:"number"` }
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Number, nil
}

func (g *githubPlatform) AddComment(ctx context.Context, issueNumber int, diagnosis, provider string) error {
	body := fmt.Sprintf("## Re-run Diagnosis\n\n**Provider:** %s\n\n%s", provider, diagnosis)
	resp, err := g.do(ctx, http.MethodPost,
		fmt.Sprintf("/repos/%s/issues/%d/comments", g.repo, issueNumber),
		map[string]string{"body": body})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
```

- [x] **Step 6: Run tests — verify PASS**

```bash
go test ./internal/platform/... -v
```

Expected: 3 tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/platform/
git commit -m "feat: add internal/platform Gitea+GitHub API clients"
```

---

## Task 6: `cmd/ci-agent` — standalone CI diagnosis agent

**Files:**

- Create: `cmd/ci-agent/providers.go`
- Create: `cmd/ci-agent/main.go`
- Create: `cmd/ci-agent/main_test.go`

- [x] **Step 1: Write failing tests**

```go
// cmd/ci-agent/main_test.go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"content":[{"type":"text","text":"root cause: missing import"}]}`))
	}))
	defer srv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "test")
	text, name, err := askWithFallback("diagnose this", srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "root cause: missing import", text)
	assert.Contains(t, name, "anthropic")
}

func TestProviderFallbackSkipsWhenNoKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	_, _, err := askWithFallback("diagnose this")
	assert.ErrorIs(t, err, errDiagnosisUnavailable)
}
```

- [x] **Step 2: Run — verify FAIL**

```bash
go test ./cmd/ci-agent/... 2>&1 | head -5
```

- [x] **Step 3: Implement `cmd/ci-agent/providers.go`**

```go
// cmd/ci-agent/providers.go
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
)

var errDiagnosisUnavailable = errors.New("all LLM providers failed or have no API keys")

const maxTokens = 1024

// askWithFallback tries Anthropic → OpenAI → Gemini.
// baseURLOverride is optional; used in tests to redirect to a mock server.
func askWithFallback(prompt string, baseURLOverride ...string) (text, provider string, err error) {
	type providerFn func(string, string) (string, error)

	anthropicBase := "https://api.anthropic.com"
	if len(baseURLOverride) > 0 && baseURLOverride[0] != "" {
		anthropicBase = baseURLOverride[0]
	}

	providers := []struct {
		name string
		key  string
		fn   providerFn
	}{
		{"anthropic/claude-sonnet-4-6", os.Getenv("ANTHROPIC_API_KEY"), func(p, _ string) (string, error) {
			return askAnthropic(p, anthropicBase)
		}},
		{"openai/gpt-4.1", os.Getenv("OPENAI_API_KEY"), askOpenAI},
		{"google/gemini-2.5-flash", os.Getenv("GEMINI_API_KEY"), askGemini},
	}

	var errs []string
	for _, prov := range providers {
		if prov.key == "" {
			continue
		}
		t, e := prov.fn(prompt, prov.key)
		if e != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", prov.name, e))
			continue
		}
		return t, prov.name, nil
	}
	if len(errs) > 0 {
		return "", "", fmt.Errorf("%w: %s", errDiagnosisUnavailable, errs)
	}
	return "", "", errDiagnosisUnavailable
}

func postJSON(url string, headers map[string]string, body any) ([]byte, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func askAnthropic(prompt, baseURL string) (string, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	raw, err := postJSON(baseURL+"/v1/messages",
		map[string]string{"x-api-key": key, "anthropic-version": "2023-06-01"},
		map[string]any{
			"model":      "claude-sonnet-4-6",
			"max_tokens": maxTokens,
			"messages":   []map[string]string{{"role": "user", "content": prompt}},
		})
	if err != nil {
		return "", err
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	for _, c := range resp.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", errors.New("no text in response")
}

func askOpenAI(prompt, key string) (string, error) {
	raw, err := postJSON("https://api.openai.com/v1/chat/completions",
		map[string]string{"Authorization": "Bearer " + key},
		map[string]any{
			"model":      "gpt-4.1",
			"max_tokens": maxTokens,
			"messages":   []map[string]string{{"role": "user", "content": prompt}},
		})
	if err != nil {
		return "", err
	}
	var resp struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("no choices in response")
	}
	return resp.Choices[0].Message.Content, nil
}

func askGemini(prompt, key string) (string, error) {
	raw, err := postJSON(
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent",
		map[string]string{"x-goog-api-key": key},
		map[string]any{"contents": []map[string]any{{"parts": []map[string]string{{"text": prompt}}}}},
	)
	if err != nil {
		return "", err
	}
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct{ Text string `json:"text"` } `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("no candidates in response")
	}
	return resp.Candidates[0].Content.Parts[0].Text, nil
}
```

- [x] **Step 4: Implement `cmd/ci-agent/main.go`**

````go
// cmd/ci-agent/main.go
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/89jobrien/devkit/internal/platform"
	"github.com/BurntSushi/toml"
)

type devkitConfig struct {
	Project struct {
		Description string `toml:"description"`
	} `toml:"project"`
	Context struct {
		Files []string `toml:"files"`
	} `toml:"context"`
}

func loadConfig() devkitConfig {
	var cfg devkitConfig
	cfg.Context.Files = []string{"CLAUDE.md", "AGENTS.md", "README.md"}
	_, _ = toml.DecodeFile(".devkit.toml", &cfg)
	return cfg
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "ERROR: required env var %s is not set\n", key)
		os.Exit(1)
	}
	return v
}

func main() {
	ciPlatform := requireEnv("CI_PLATFORM")
	repo := requireEnv("REPO")
	runID := requireEnv("RUN_ID")
	sha := requireEnv("COMMIT_SHA")

	var token string
	switch ciPlatform {
	case "gitea":
		token = requireEnv("CI_AGENT_TOKEN")
	case "github":
		token = requireEnv("GITHUB_TOKEN")
	default:
		fmt.Fprintf(os.Stderr, "ERROR: unknown CI_PLATFORM %q\n", ciPlatform)
		os.Exit(1)
	}

	gitBaseURL := os.Getenv("GITEA_URL") // only needed for gitea

	ctx := context.Background()
	p, err := platform.New(ciPlatform, repo, runID, sha, token, gitBaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}

	fmt.Printf("=== CI Diagnosis Agent — run %s ===\n", runID)

	logs, err := p.FetchFailedJobLogs(ctx, runID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR fetching job logs:", err)
		os.Exit(1)
	}
	if len(logs) == 0 {
		fmt.Println("No failed jobs found — nothing to diagnose.")
		return
	}

	failedNames := make([]string, len(logs))
	for i, l := range logs {
		failedNames[i] = l.Name
	}
	fmt.Printf("Failed jobs: %v\n", failedNames)

	cfg := loadConfig()

	// Build prompt sections
	var sections []string
	for _, l := range logs {
		sections = append(sections, fmt.Sprintf("### Job: %s\n```\n%s\n```", l.Name, l.Log))
	}
	logsText := strings.Join(sections, "\n\n")

	var ctxParts []string
	if cfg.Project.Description != "" {
		ctxParts = append(ctxParts, "**Project:** "+cfg.Project.Description)
	}
	for _, f := range cfg.Context.Files {
		data, err := os.ReadFile(f)
		if err == nil {
			ctxParts = append(ctxParts, fmt.Sprintf("**%s:**\n%s", f, string(data)))
		}
	}

	prompt := fmt.Sprintf("You are a CI expert. The following jobs failed.\n\n%s\n\n%s\n\nAnalyze and provide:\n1. **Root cause** — what exactly failed and why\n2. **Fix** — the minimal change needed\n3. **Confidence** — high/medium/low",
		strings.Join(ctxParts, "\n\n"), logsText)

	_ = p.SetCommitStatus(ctx, "pending", "CI diagnosis in progress...")

	diagnosis, provider, err := askWithFallback(prompt)
	if err != nil {
		fmt.Println("All LLM providers failed:", err)
		diagnosis = fmt.Sprintf("All LLM providers failed.\n\n%s", logsText)
		provider = "none"
	}

	fmt.Printf("\n%s\nDIAGNOSIS (via %s)\n%s\n%s\n", strings.Repeat("=", 60), provider, strings.Repeat("=", 60), diagnosis)

	state := "failure"
	if provider == "none" {
		state = "error"
	}
	firstLine := diagnosis
	if i := strings.Index(diagnosis, "\n"); i > 0 {
		firstLine = diagnosis[:i]
	}
	if len(firstLine) > 120 {
		firstLine = firstLine[:120]
	}
	_ = p.SetCommitStatus(ctx, state, "Diagnosed: "+firstLine)

	_ = p.EnsureLabelExists(ctx)
	existing, found, _ := p.FindIssueForCommit(ctx, sha)
	if found {
		fmt.Printf("Appending to existing issue #%d\n", existing)
		_ = p.AddComment(ctx, existing, diagnosis, provider)
	} else {
		num, err := p.CreateIssue(ctx, sha, diagnosis, provider, failedNames, runID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Warning: could not create issue:", err)
		} else {
			fmt.Printf("Opened issue #%d\n", num)
		}
	}
}
````

- [x] **Step 5: Run tests — verify PASS**

```bash
go test ./cmd/ci-agent/... -v
```

Expected: 2 tests pass.

- [x] **Step 6: Build check**

```bash
go build ./cmd/ci-agent/
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add cmd/ci-agent/
git commit -m "feat: add cmd/ci-agent standalone CI diagnosis agent"
```

---

## Task 7: `internal/council`, `internal/review`, `internal/meta`

**Files:**

- Create: `internal/council/council.go`
- Create: `internal/council/council_test.go`
- Create: `internal/review/review.go`
- Create: `internal/review/review_test.go`
- Create: `internal/meta/meta.go`
- Create: `internal/meta/meta_test.go`

- [x] **Step 1: Write failing tests for council**

```go
// internal/council/council_test.go
package council_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/council"
	"github.com/89jobrien/devkit/internal/loop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRunner struct{ response string }

func (s stubRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	return s.response, nil
}

func TestRunCoreRolesConcurrently(t *testing.T) {
	runner := stubRunner{response: "**Health Score**: 0.8\n## Summary\nLooks good."}
	result, err := council.Run(context.Background(), council.Config{
		Base:     "main",
		Mode:     "core",
		Diff:     "diff --git a/foo.go",
		Commits:  "abc123 add foo",
		Runner:   runner,
	})
	require.NoError(t, err)
	assert.Len(t, result.RoleOutputs, 3)
	assert.NotEmpty(t, result.RoleOutputs["strict-critic"])
}

func TestRunExtensiveHasFiveRoles(t *testing.T) {
	runner := stubRunner{response: "**Health Score**: 0.7\n## Summary\nOK."}
	result, err := council.Run(context.Background(), council.Config{
		Base: "main", Mode: "extensive", Diff: "diff", Commits: "abc", Runner: runner,
	})
	require.NoError(t, err)
	assert.Len(t, result.RoleOutputs, 5)
}

func TestMetaScore(t *testing.T) {
	outputs := map[string]string{
		"strict-critic":    "**Health Score**: 0.6",
		"creative-explorer": "**Health Score**: 0.9",
		"general-analyst":  "**Health Score**: 0.8",
	}
	score := council.MetaScore(outputs)
	// strict-critic weight 1.5x: (0.6*1.5 + 0.9 + 0.8) / (1.5+1+1) = 2.6/3.5 ≈ 0.743
	assert.InDelta(t, 0.743, score, 0.01)
}
```

- [x] **Step 2: Run — verify FAIL**

```bash
go test ./internal/council/... 2>&1 | head -5
```

- [x] **Step 3: Implement `internal/council/council.go`**

````go
// internal/council/council.go
package council

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"
)

// Runner abstracts the LLM call for testability.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RealRunner wraps loop.RunAgent using the Anthropic client.
// Constructed by cmd/devkit; council itself is client-agnostic.
type RealRunner struct {
	RunFn func(ctx context.Context, prompt string) (string, error)
}

func (r RealRunner) Run(ctx context.Context, prompt string, _ []string) (string, error) {
	return r.RunFn(ctx, prompt)
}

// Config holds parameters for a council run.
type Config struct {
	Base    string
	Mode    string // "core" or "extensive"
	Diff    string
	Commits string
	Runner  Runner
}

// Result holds all role outputs and the synthesis.
type Result struct {
	RoleOutputs map[string]string
	Synthesis   string
}

var roles = map[string]struct{ label, persona string }{
	"strict-critic": {"Strict Critic",
		"You are the STRICT CRITIC. Be conservative and demanding. Health Score 0.0–1.0 (only near-perfect scores above 0.85). Include: **Health Score**, **Summary**, **Key Observations**, **Risks Identified**, **Recommendations**."},
	"creative-explorer": {"Creative Explorer",
		"You are the CREATIVE EXPLORER. Be optimistic and inventive. Include: **Health Score**, **Summary**, **Innovation Opportunities**, **Architectural Potential**, **Recommendations**."},
	"general-analyst": {"General Analyst",
		"You are the GENERAL ANALYST. Be balanced and evidence-based. Include: **Health Score**, **Summary**, **Progress Indicators**, **Work Patterns**, **Gaps**, **Recommendations**."},
	"security-reviewer": {"Security Reviewer",
		"You are the SECURITY REVIEWER. Focus on attack surface: injection, traversal, auth bypasses, unsafe patterns. Include: **Health Score** (any critical vuln = max 0.4), **Summary**, **Findings** (critical/high/medium/low), **Recommendations**."},
	"performance-analyst": {"Performance Analyst",
		"You are the PERFORMANCE ANALYST. Focus on allocations, blocking calls, algorithmic complexity. Include: **Health Score**, **Summary**, **Bottlenecks**, **Optimization Opportunities**, **Recommendations**."},
}

var coreRoles = []string{"strict-critic", "creative-explorer", "general-analyst"}
var extensiveRoles = append(coreRoles, "security-reviewer", "performance-analyst")

// Run executes all council roles concurrently and returns their outputs.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	roleKeys := coreRoles
	if cfg.Mode == "extensive" {
		roleKeys = extensiveRoles
	}

	context_ := fmt.Sprintf("Branch vs %s\n\nCommits:\n%s\n\nDiff:\n```diff\n%s\n```", cfg.Base, cfg.Commits, cfg.Diff)

	outputs := make(map[string]string, len(roleKeys))
	var mu = make(chan struct{}, 1)
	mu <- struct{}{}

	g, ctx := errgroup.WithContext(ctx)
	for _, key := range roleKeys {
		key := key
		role := roles[key]
		g.Go(func() error {
			prompt := fmt.Sprintf("%s\n\nAnalyse this branch. Read relevant source files to support your findings.\n\n%s", role.persona, context_)
			out, err := cfg.Runner.Run(ctx, prompt, []string{"Read", "Glob", "Grep"})
			if err != nil {
				return fmt.Errorf("role %s: %w", key, err)
			}
			<-mu
			outputs[key] = out
			mu <- struct{}{}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &Result{RoleOutputs: outputs}, nil
}

var healthScoreRe = regexp.MustCompile(`(?i)\*\*Health Score\*\*[:\s]*([\d.]+)`)

// ParseHealthScore extracts the first health score from role output text.
func ParseHealthScore(text string) float64 {
	m := healthScoreRe.FindStringSubmatch(text)
	if m == nil {
		return 0.5
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(m[1]), 64)
	return v
}

// MetaScore computes the weighted meta-score (Strict Critic 1.5×, others 1.0×).
func MetaScore(outputs map[string]string) float64 {
	weights := map[string]float64{
		"strict-critic":     1.5,
		"creative-explorer": 1.0,
		"general-analyst":   1.0,
		"security-reviewer": 1.0,
		"performance-analyst": 1.0,
	}
	var sum, totalW float64
	for key, out := range outputs {
		w := weights[key]
		if w == 0 {
			w = 1.0
		}
		sum += ParseHealthScore(out) * w
		totalW += w
	}
	if totalW == 0 {
		return 0
	}
	return sum / totalW
}

// Synthesize runs a synthesis agent over all role outputs.
func Synthesize(ctx context.Context, outputs map[string]string, cfg Config, runner Runner) (string, error) {
	var parts []string
	for key, out := range outputs {
		parts = append(parts, fmt.Sprintf("### %s\n%s", roles[key].label, out))
	}
	councilText := strings.Join(parts, "\n\n---\n\n")

	prompt := fmt.Sprintf(`Synthesize this multi-role council review into a final verdict.

Required sections:
**Health Scores** — list each role's score, compute meta-score (Strict Critic 1.5× weight).
**Areas of Consensus** — findings where 2+ roles agree.
**Areas of Tension** — dialectic format: "[Role A] sees [X], AND [Role B] sees [Y], suggesting [resolution]."
**Balanced Recommendations** — top 3–5 ranked actions.
**Branch Health** — one of: Good / Needs work / Significant issues — with one-line justification.

Branch context vs %s:
%s

Council findings:
%s`, cfg.Base, fmt.Sprintf("Commits:\n%s\nDiff (first 2000 chars):\n%s", cfg.Commits, cfg.Diff[:min(2000, len(cfg.Diff))]), councilText)

	return runner.Run(ctx, prompt, nil)
}

// Note: min() is a Go 1.21+ builtin — no local helper needed with go1.23
````

- [x] **Step 4: Write and implement `internal/review/review.go`**

````go
// internal/review/review.go
package review

import (
	"context"
	"fmt"
)

// Runner abstracts the LLM call.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// Config holds parameters for a diff review.
type Config struct {
	Base   string
	Diff   string
	Focus  string
	Runner Runner
}

// Run executes a single-agent diff review and returns the output.
func Run(ctx context.Context, cfg Config) (string, error) {
	if cfg.Focus == "" {
		cfg.Focus = "- Security: injection, traversal, auth bypasses\n- Correctness: error handling, breaking changes\n- Unsafe patterns"
	}
	prompt := fmt.Sprintf(`Review this diff.

Focus areas:
%s

For each issue: file + line, severity (critical/major/minor), and a concrete fix.
If no issues, say so clearly.

` + "```diff\n%s\n```", cfg.Focus, cfg.Diff)

	return cfg.Runner.Run(ctx, prompt, []string{"Read", "Glob", "Grep"})
}
````

```go
// internal/review/review_test.go
package review_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRunner struct{ response string }

func (s stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, nil
}

func TestReviewReturnsOutput(t *testing.T) {
	r := stubRunner{response: "No issues found."}
	result, err := review.Run(context.Background(), review.Config{
		Base:   "main",
		Diff:   "diff --git a/main.go b/main.go",
		Runner: r,
	})
	require.NoError(t, err)
	assert.Equal(t, "No issues found.", result)
}

func TestReviewUsesCustomFocus(t *testing.T) {
	var capturedPrompt string
	r := review.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		capturedPrompt = prompt
		return "ok", nil
	})
	_, _ = review.Run(context.Background(), review.Config{
		Base: "main", Diff: "diff", Focus: "- Rust unsafe", Runner: r,
	})
	assert.Contains(t, capturedPrompt, "Rust unsafe")
}
```

Add `RunnerFunc` to review.go:

```go
// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)
func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) { return f(ctx, prompt, tools) }
```

- [x] **Step 5: Write and implement `internal/meta/meta.go`**

````go
// internal/meta/meta.go
package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"
)

// Runner abstracts the LLM call.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// AgentSpec is output from the designer agent.
type AgentSpec struct {
	Name   string   `json:"name"`
	Role   string   `json:"role"`
	Prompt string   `json:"prompt"`
	Tools  []string `json:"tools"`
}

// Result holds the design plan, worker outputs, and synthesis.
type Result struct {
	Plan    []AgentSpec
	Outputs map[string]string
	Summary string
}

// Run executes the full meta-agent flow: design → parallel workers → synthesis.
func Run(ctx context.Context, task, repoContext, sdkDocs string, runner Runner) (*Result, error) {
	// Phase 1: design
	plan, err := design(ctx, task, repoContext, sdkDocs, runner)
	if err != nil {
		return nil, fmt.Errorf("design: %w", err)
	}

	// Phase 2: parallel workers
	outputs := make(map[string]string, len(plan))
	mu := make(chan struct{}, 1)
	mu <- struct{}{}

	g, ctx := errgroup.WithContext(ctx)
	for _, spec := range plan {
		spec := spec
		g.Go(func() error {
			out, err := runner.Run(ctx, spec.Prompt, spec.Tools)
			if err != nil {
				return fmt.Errorf("agent %s: %w", spec.Name, err)
			}
			<-mu
			outputs[spec.Name] = out
			mu <- struct{}{}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Phase 3: synthesis
	summary, err := synthesize(ctx, task, outputs, runner)
	if err != nil {
		return nil, fmt.Errorf("synthesis: %w", err)
	}

	return &Result{Plan: plan, Outputs: outputs, Summary: summary}, nil
}

func design(ctx context.Context, task, repoContext, sdkDocs string, runner Runner) ([]AgentSpec, error) {
	prompt := fmt.Sprintf(`You are a meta-agent designer. Design the smallest set of parallel agents to accomplish this task.

Output ONLY a valid JSON array with schema:
[{"name":"kebab-name","role":"one sentence","prompt":"complete self-contained prompt","tools":["Read","Glob","Grep"]}]

Rules:
- 2–5 agents, each with a distinct non-overlapping concern
- Prompts must be fully self-contained
- Available tools: Read, Glob, Grep (reads); Bash, Write, Edit (modifications)
- Only grant Write/Edit/Bash when genuinely needed
- Do NOT include a synthesis agent

## Task
%s

## Repo context
%s

## SDK docs
%s`, task, repoContext[:min(4000, len(repoContext))], sdkDocs[:min(6000, len(sdkDocs))])

	raw, err := runner.Run(ctx, prompt, nil)
	if err != nil {
		return nil, err
	}

	// Strip markdown fences if present
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		lines := strings.SplitN(raw, "\n", 2)
		if len(lines) == 2 {
			raw = strings.TrimSuffix(strings.TrimSpace(lines[1]), "```")
		}
	}

	var plan []AgentSpec
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		// Fallback: single analyst agent
		return []AgentSpec{{
			Name: "analyst", Role: "General analysis",
			Prompt: task, Tools: []string{"Read", "Glob", "Grep"},
		}}, nil
	}
	return plan, nil
}

func synthesize(ctx context.Context, task string, outputs map[string]string, runner Runner) (string, error) {
	var parts []string
	for name, out := range outputs {
		parts = append(parts, fmt.Sprintf("### %s\n%s", name, out))
	}
	combined := strings.Join(parts, "\n\n---\n\n")

	prompt := fmt.Sprintf(`Synthesize outputs from parallel agents into a coherent report.

Required sections:
**Summary** — 2–3 sentences: what the agents found and overall verdict
**Key Findings** — deduplicated bullets grouped by theme
**Recommended Actions** — ranked list with who/what/why
**Open Questions** — anything unresolved

Original task: %s

Agent outputs:
%s`, task, combined)

	return runner.Run(ctx, prompt, nil)
}

// Note: min() is a Go 1.21+ builtin — no local helper needed with go1.23
````

```go
// internal/meta/meta_test.go
package meta_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/meta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRunner struct {
	designResponse string
	workerResponse string
}

func (s *stubRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	// Return design JSON on first call (detected by presence of "JSON array"), worker response otherwise
	if len(s.designResponse) > 0 && s.designResponse != "used" {
		r := s.designResponse
		s.designResponse = "used"
		return r, nil
	}
	return s.workerResponse, nil
}

func TestRunDesignsAndExecutes(t *testing.T) {
	runner := &stubRunner{
		designResponse: `[{"name":"checker","role":"check stuff","prompt":"check the code","tools":["Read"]}]`,
		workerResponse: "found 2 issues",
	}
	result, err := meta.Run(context.Background(), "audit the code", "repo context", "sdk docs", runner)
	require.NoError(t, err)
	require.Len(t, result.Plan, 1)
	assert.Equal(t, "checker", result.Plan[0].Name)
	assert.Contains(t, result.Outputs["checker"], "found 2 issues")
}

func TestRunFallsBackOnInvalidJSON(t *testing.T) {
	runner := &stubRunner{
		designResponse: "not valid json",
		workerResponse: "analysis complete",
	}
	result, err := meta.Run(context.Background(), "do something", "", "", runner)
	require.NoError(t, err)
	assert.Len(t, result.Plan, 1) // fallback single agent
	assert.Equal(t, "analyst", result.Plan[0].Name)
}
```

- [x] **Step 6: Run all three tests — verify PASS**

```bash
go test ./internal/council/... ./internal/review/... ./internal/meta/... -v
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/council/ internal/review/ internal/meta/
git commit -m "feat: add council, review, and meta-agent internals"
```

---

## Task 8: `cmd/devkit` — main CLI binary

**Files:**

- Modify: `cmd/devkit/main.go`
- Create: `cmd/devkit/config.go`
- Create: `cmd/devkit/runner.go`

- [x] **Step 1: Implement `cmd/devkit/config.go`**

```go
// cmd/devkit/config.go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Project struct {
		Name        string   `toml:"name"`
		Description string   `toml:"description"`
		Version     string   `toml:"version"`
		InstallDate string   `toml:"install_date"`
		CIPlatforms []string `toml:"ci_platforms"`
	} `toml:"project"`
	Context struct {
		Files []string `toml:"files"`
	} `toml:"context"`
	Review struct {
		Focus string `toml:"focus"`
	} `toml:"review"`
	Components struct {
		Council  bool `toml:"council"`
		Review   bool `toml:"review"`
		Meta     bool `toml:"meta"`
		CIAgent  bool `toml:"ci_agent"`
	} `toml:"components"`
}

// LoadConfig finds and parses the nearest .devkit.toml walking up from cwd.
func LoadConfig() (*Config, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for {
		path := filepath.Join(dir, ".devkit.toml")
		if _, err := os.Stat(path); err == nil {
			var cfg Config
			if _, err := toml.DecodeFile(path, &cfg); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", path, err)
			}
			return &cfg, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil, fmt.Errorf(".devkit.toml not found (run install.sh first)")
}
```

- [x] **Step 2: Implement `cmd/devkit/runner.go`**

```go
// cmd/devkit/runner.go
package main

import (
	"context"
	"os"

	"github.com/89jobrien/devkit/internal/loop"
	"github.com/89jobrien/devkit/internal/tools"
	"github.com/anthropics/anthropic-sdk-go"
)

// agentRunner implements council.Runner, review.Runner, and meta.Runner
// using the real Anthropic client with Read/Glob/Grep tools.
type agentRunner struct {
	client *anthropic.Client
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

	// Filter to requested tools (or use all if none specified)
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
```

- [x] **Step 3: Implement `cmd/devkit/main.go`**

```go
// cmd/devkit/main.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/89jobrien/devkit/internal/council"
	devlog "github.com/89jobrien/devkit/internal/log"
	"github.com/89jobrien/devkit/internal/meta"
	"github.com/89jobrien/devkit/internal/review"
	"github.com/spf13/cobra"
)

func gitDiff(base string) string {
	out, err := exec.Command("git", "diff", base+"...HEAD").Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		out, _ = exec.Command("git", "diff", "HEAD").Output()
	}
	return string(out)
}

func gitLog(base string) string {
	out, _ := exec.Command("git", "log", base+"...HEAD", "--oneline").Output()
	return string(out)
}

func main() {
	root := &cobra.Command{
		Use:   "devkit",
		Short: "AI-powered dev workflow toolkit",
	}

	// council
	var councilBase, councilMode string
	var councilNoSynth bool
	councilCmd := &cobra.Command{
		Use:   "council",
		Short: "Multi-role branch analysis",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			diff := gitDiff(councilBase)
			commits := gitLog(councilBase)
			runner := newAgentRunner()
			sha := devlog.GitShortSHA()

			id := devlog.Start("council", map[string]string{"base": councilBase, "mode": councilMode})
			start := time.Now()

			result, err := council.Run(cmd.Context(), council.Config{
				Base: councilBase, Mode: councilMode,
				Diff: diff, Commits: commits,
				Runner: council.RealRunner{RunFn: func(ctx context.Context, prompt string) (string, error) {
					return runner.Run(ctx, prompt, []string{"Read", "Glob", "Grep"})
				}},
			})
			if err != nil {
				return err
			}

			var allOutput strings.Builder
			for key, out := range result.RoleOutputs {
				fmt.Printf("\n──── %s ────\n%s\n", key, out)
				allOutput.WriteString(fmt.Sprintf("## %s\n%s\n\n", key, out))
			}

			if !councilNoSynth {
				synthesis, err := council.Synthesize(cmd.Context(), result.RoleOutputs, council.Config{
					Base: councilBase, Diff: diff, Commits: commits,
				}, council.RealRunner{RunFn: func(ctx context.Context, prompt string) (string, error) {
					return runner.Run(ctx, prompt, nil)
				}})
				if err != nil {
					return err
				}
				fmt.Printf("\n──── SYNTHESIS ────\n%s\n", synthesis)
				allOutput.WriteString(fmt.Sprintf("## Synthesis\n%s\n", synthesis))
			}

			score := council.MetaScore(result.RoleOutputs)
			fmt.Printf("\nMeta Health Score: %.0f%%\n", score*100)

			devlog.Complete(id, "council", map[string]string{"base": councilBase, "mode": councilMode},
				allOutput.String(), time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "council", allOutput.String(), map[string]string{
				"base": councilBase, "mode": councilMode,
			})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	councilCmd.Flags().StringVar(&councilBase, "base", "main", "Base branch/ref")
	councilCmd.Flags().StringVar(&councilMode, "mode", "core", "core or extensive")
	councilCmd.Flags().BoolVar(&councilNoSynth, "no-synthesis", false, "Skip synthesis")

	// review
	var reviewBase string
	reviewCmd := &cobra.Command{
		Use:   "review",
		Short: "AI diff review",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			diff := gitDiff(reviewBase)
			if strings.TrimSpace(diff) == "" {
				fmt.Printf("No changes versus %s — nothing to review.\n", reviewBase)
				return nil
			}

			runner := newAgentRunner()
			sha := devlog.GitShortSHA()
			id := devlog.Start("review", map[string]string{"base": reviewBase})
			start := time.Now()

			result, err := review.Run(cmd.Context(), review.Config{
				Base:  reviewBase,
				Diff:  diff,
				Focus: cfg.Review.Focus,
				Runner: review.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return runner.Run(ctx, prompt, ts)
				}),
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "review", map[string]string{"base": reviewBase}, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "review", result, map[string]string{"base": reviewBase})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	reviewCmd.Flags().StringVar(&reviewBase, "base", "main", "Base branch/ref")

	// meta
	var metaNoSynth, metaRefreshDocs bool
	metaCmd := &cobra.Command{
		Use:   "meta [task]",
		Short: "Design and run parallel agents for any task",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			task := ""
			if len(args) > 0 {
				task = args[0]
			} else {
				info, _ := os.Stdin.Stat()
				if info.Mode()&os.ModeCharDevice == 0 {
					var sb strings.Builder
					buf := make([]byte, 4096)
					for {
						n, err := os.Stdin.Read(buf)
						sb.Write(buf[:n])
						if err != nil {
							break
						}
					}
					task = strings.TrimSpace(sb.String())
				}
			}
			if task == "" {
				return fmt.Errorf("provide a task as argument or via stdin")
			}

			cfg, _ := LoadConfig()
			if cfg != nil && cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			repoContext := gatherRepoContext()
			sdkDocs := fetchSDKDocs(metaRefreshDocs)
			runner := newAgentRunner()
			sha := devlog.GitShortSHA()

			id := devlog.Start("meta", map[string]string{"task": task[:min(80, len(task))]})
			start := time.Now()

			result, err := meta.Run(cmd.Context(), task, repoContext, sdkDocs, meta.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
				return runner.Run(ctx, prompt, ts)
			}))
			if err != nil {
				return err
			}

			var allOutput strings.Builder
			for name, out := range result.Outputs {
				fmt.Printf("\n──── %s ────\n%s\n", name, out)
				allOutput.WriteString(fmt.Sprintf("## %s\n%s\n\n", name, out))
			}
			if !metaNoSynth {
				fmt.Printf("\n──── SYNTHESIS ────\n%s\n", result.Summary)
				allOutput.WriteString(fmt.Sprintf("## Synthesis\n%s\n", result.Summary))
			}

			devlog.Complete(id, "meta", map[string]string{"task": task[:min(80, len(task))]},
				allOutput.String(), time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "meta", allOutput.String(), map[string]string{"task": task[:min(80, len(task))]})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	metaCmd.Flags().BoolVar(&metaNoSynth, "no-synthesis", false, "Skip synthesis")
	metaCmd.Flags().BoolVar(&metaRefreshDocs, "refresh-docs", false, "Force re-fetch SDK docs")

	root.AddCommand(councilCmd, reviewCmd, metaCmd)
	if err := root.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}

// Note: min() is a Go 1.21+ builtin — no local helper needed with go1.23
```

- [x] **Step 4: Add `gatherRepoContext` and `fetchSDKDocs` helpers to `cmd/devkit/main.go`**

Add imports `"io"`, `"net/http"`, `"regexp"` to the import block. Append to main.go:

```go
func gatherRepoContext() string {
	run := func(args ...string) string {
		out, _ := exec.Command(args[0], args[1:]...).Output()
		return string(out)
	}
	var sb strings.Builder
	for _, f := range []string{"CLAUDE.md", "AGENTS.md", "README.md"} {
		if data, err := os.ReadFile(f); err == nil {
			sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", f, string(data[:min(2000, len(data))])))
		}
	}
	sb.WriteString("## Recent commits\n" + run("git", "log", "--oneline", "-20"))
	sb.WriteString("\n## Working tree\n" + run("git", "status", "--short"))
	// Top-150 tracked file paths (excludes build artifacts)
	allFiles := run("git", "ls-files")
	lines := strings.Split(strings.TrimSpace(allFiles), "\n")
	if len(lines) > 150 {
		lines = lines[:150]
	}
	sb.WriteString("\n## Structure (first 150 paths)\n" + strings.Join(lines, "\n"))
	return sb.String()
}

var sdkDocURLs = []string{
	"https://docs.anthropic.com/en/docs/claude-code/sdk",
	"https://docs.anthropic.com/en/docs/claude-code/sdk/sdk-python",
}

func fetchSDKDocs(forceRefresh bool) string {
	home, _ := os.UserHomeDir()
	cacheDir := home + "/.dev-agents/cache"
	cachePath := cacheDir + "/sdk-docs.md"

	if !forceRefresh {
		if info, err := os.Stat(cachePath); err == nil {
			if time.Since(info.ModTime()) < 24*time.Hour {
				data, _ := os.ReadFile(cachePath)
				return string(data)
			}
		}
	}

	_ = os.MkdirAll(cacheDir, 0o755)

	var sections []string
	client := &http.Client{Timeout: 15 * time.Second}
	for _, url := range sdkDocURLs {
		resp, err := client.Get(url)
		if err != nil {
			sections = append(sections, fmt.Sprintf("<!-- failed to fetch %s: %v -->", url, err))
			continue
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		text := htmlToText(string(raw))
		if len(text) > 20000 {
			text = text[:20000]
		}
		sections = append(sections, fmt.Sprintf("<!-- %s -->\n%s", url, text))
	}

	content := strings.Join(sections, "\n\n---\n\n")
	_ = os.WriteFile(cachePath, []byte(content), 0o644)
	return content
}

// htmlToText strips HTML tags, returning visible text content.
func htmlToText(html string) string {
	// Remove script/style blocks
	re := regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	html = re.ReplaceAllString(html, "")
	// Remove all tags
	tags := regexp.MustCompile(`<[^>]+>`)
	text := tags.ReplaceAllString(html, " ")
	// Collapse whitespace
	ws := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(ws.ReplaceAllString(text, " "))
}
```

- [x] **Step 5: Add `RunnerFunc` adapter to `meta` package** (add to `internal/meta/meta.go`):

```go
// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)
func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) { return f(ctx, prompt, tools) }
```

- [x] **Step 6: Build the binary — verify it compiles**

```bash
go build ./cmd/devkit/
./devkit --help
```

Expected: help text with council/review/meta subcommands.

- [x] **Step 7: Run full test suite**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 8: Commit**

```bash
git add cmd/devkit/
git commit -m "feat: add cmd/devkit CLI binary with council/review/meta"
```

---

## Task 9: Shell scripts + CI templates

**Files:**

- Create: `ci/gitea.yml`
- Create: `ci/github.yml`
- Create: `install.sh`
- Create: `upgrade.sh`

- [x] **Step 1: Create `ci/gitea.yml`**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: test
        run: YOUR_TEST_COMMAND

  diagnose:
    name: Diagnose Failures
    runs-on: ubuntu-latest
    needs: [test]
    if: failure()
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
      - name: run diagnosis agent
        env:
          CI_PLATFORM: gitea
          GITEA_URL: http://YOUR_GITEA_HOST:3000
          CI_AGENT_TOKEN: ${{ secrets.CI_AGENT_TOKEN }}
          REPO: ${{ gitea.repository }}
          RUN_ID: ${{ gitea.run_id }}
          COMMIT_SHA: ${{ gitea.sha }}
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
          GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}
        run: go run github.com/89jobrien/devkit/cmd/ci-agent@DEVKIT_VERSION
```

- [x] **Step 2: Create `ci/github.yml`**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: test
        run: YOUR_TEST_COMMAND

  diagnose:
    name: Diagnose Failures
    runs-on: ubuntu-latest
    needs: [test]
    if: failure()
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
      - name: run diagnosis agent
        env:
          CI_PLATFORM: github
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          REPO: ${{ github.repository }}
          RUN_ID: ${{ github.run_id }}
          COMMIT_SHA: ${{ github.sha }}
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
          GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}
        run: go run github.com/89jobrien/devkit/cmd/ci-agent@DEVKIT_VERSION
```

- [x] **Step 3: Create `install.sh`** (use `gum` for interactive prompts per tooling preferences)

```bash
#!/usr/bin/env bash
set -euo pipefail

DEVKIT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERSION="$(cat "$DEVKIT_DIR/VERSION")"

if [[ -f ".devkit.toml" ]]; then
  echo "Error: devkit already installed (.devkit.toml found)."
  echo "Run $DEVKIT_DIR/upgrade.sh to update CI templates."
  exit 1
fi

echo "devkit v$VERSION — install"
echo ""

# Prompts
PROJECT_NAME=$(gum input --placeholder "project name" --value "$(basename "$PWD")")
DESCRIPTION=$(gum input --placeholder "one-line project description")
CI_PLATFORM=$(gum choose --header "CI platform" "gitea" "github" "both")

# Component selection
COMPONENTS=$(gum choose --no-limit --header "Components to install (space to toggle, enter to confirm)" \
  "council" "review" "meta" "ci-agent" | tr '\n' ' ')
# Default to all if none selected
[[ -z "$COMPONENTS" ]] && COMPONENTS="council review meta ci-agent"

# Language detection
TEST_CMD="YOUR_TEST_COMMAND"
REVIEW_EXTRAS=""
if [[ -f "Cargo.toml" ]]; then
  TEST_CMD="cargo test --workspace"
  REVIEW_EXTRAS="- Rust: path traversal, unsafe block soundness"
elif [[ -f "pyproject.toml" ]] || [[ -f "setup.py" ]]; then
  TEST_CMD="uv run pytest"
  REVIEW_EXTRAS="- Python: injection, deserialization safety"
elif [[ -f "package.json" ]]; then
  TEST_CMD="bun test"
  REVIEW_EXTRAS="- JS/TS: prototype pollution, XSS"
elif [[ -f "go.mod" ]]; then
  TEST_CMD="go test ./..."
  REVIEW_EXTRAS="- Go: nil dereference, goroutine leaks"
fi

# Write .devkit.toml
cat > .devkit.toml <<TOML
[project]
name        = "$PROJECT_NAME"
description = "$DESCRIPTION"
version     = "$VERSION"
install_date = "$(date +%Y-%m-%d)"
ci_platforms = [$(echo "$CI_PLATFORM" | sed 's/gitea/"gitea"/;s/github/"github"/;s/both/"gitea", "github"/')]

[context]
files = ["CLAUDE.md", "AGENTS.md", "README.md"]

[review]
focus = """
- Security: injection, auth bypasses, dependency risks
- Correctness: error handling, breaking API changes
$REVIEW_EXTRAS
"""

[components]
council  = $(echo "$COMPONENTS" | grep -q "council"  && echo "true" || echo "false")
review   = $(echo "$COMPONENTS" | grep -q "review"   && echo "true" || echo "false")
meta     = $(echo "$COMPONENTS" | grep -q "meta"     && echo "true" || echo "false")
ci_agent = $(echo "$COMPONENTS" | grep -q "ci-agent" && echo "true" || echo "false")
TOML

echo "✓ .devkit.toml written"

# CI templates
install_ci() {
  local platform="$1"
  local src="$DEVKIT_DIR/ci/${platform}.yml"
  local dest_dir=".${platform}/workflows"
  local dest="$dest_dir/ci.yml"
  if [[ -f "$dest" ]]; then
    echo "Warning: $dest already exists — skipping"
    return
  fi
  mkdir -p "$dest_dir"
  sed "s/YOUR_TEST_COMMAND/$TEST_CMD/g; s/DEVKIT_VERSION/v$VERSION/g" "$src" > "$dest"
  echo "✓ $dest written"
}

case "$CI_PLATFORM" in
  gitea) install_ci gitea ;;
  github) install_ci github ;;
  both) install_ci gitea; install_ci github ;;
esac

# Justfile snippet
echo ""
echo "──── Add to your Justfile ────"
cat <<'JUST'
council base="main" mode="core":
    devkit council --base {{base}} --mode {{mode}}

review base="main":
    devkit review --base {{base}}

meta task:
    devkit meta "{{task}}"
JUST
```

```bash
chmod +x install.sh
```

- [x] **Step 4: Create `upgrade.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail

DEVKIT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ ! -f ".devkit.toml" ]]; then
  echo "Error: .devkit.toml not found — not a devkit project."
  exit 1
fi

NEW_VERSION="$(cat "$DEVKIT_DIR/VERSION")"
CURRENT_VERSION=$(grep '^version' .devkit.toml | head -1 | sed 's/.*= *"\(.*\)"/\1/')

echo "devkit upgrade: $CURRENT_VERSION → $NEW_VERSION"

# Simple semver comparison (downgrade warning)
if [[ "$NEW_VERSION" < "$CURRENT_VERSION" ]]; then
  gum confirm "New version ($NEW_VERSION) is older than installed ($CURRENT_VERSION). Proceed?" || exit 0
fi

# Install new binary
go install github.com/89jobrien/devkit/cmd/devkit@latest
echo "✓ devkit binary updated"

# Regenerate CI templates
CI_PLATFORMS=$(grep 'ci_platforms' .devkit.toml | grep -oP '"[^"]*"' | tr -d '"')
for platform in $CI_PLATFORMS; do
  dest=".${platform}/workflows/ci.yml"
  src="$DEVKIT_DIR/ci/${platform}.yml"
  if [[ -f "$dest" ]]; then
    TEST_CMD=$(grep 'run:' "$dest" | grep -v 'go run' | grep -v 'setup-go' | head -1 | sed 's/.*run: //')
    gum confirm "Regenerate $dest?" || continue
  fi
  mkdir -p ".${platform}/workflows"
  sed "s/YOUR_TEST_COMMAND/${TEST_CMD:-YOUR_TEST_COMMAND}/g; s/DEVKIT_VERSION/v$NEW_VERSION/g" "$src" > "$dest"
  echo "✓ $dest updated"
done

# Notify about new components not in .devkit.toml
ALL_COMPONENTS="council review meta ci_agent"
for comp in $ALL_COMPONENTS; do
  if ! grep -q "^${comp} *= *true" .devkit.toml; then
    echo "Notice: component '$comp' is available in devkit v$NEW_VERSION but is set to false in .devkit.toml"
  fi
done

# Update version in .devkit.toml
TODAY=$(date +%Y-%m-%d)
sed -i.bak "s/^version *= *\".*\"/version = \"$NEW_VERSION\"/" .devkit.toml
sed -i.bak "s/^install_date *= *\".*\"/install_date = \"$TODAY\"/" .devkit.toml
rm -f .devkit.toml.bak
echo "✓ .devkit.toml updated to v$NEW_VERSION"
```

```bash
chmod +x upgrade.sh
```

- [x] **Step 5: Verify scripts are executable and CI templates parse cleanly**

```bash
bash -n install.sh && echo "install.sh: ok"
bash -n upgrade.sh && echo "upgrade.sh: ok"
python3 -c "import yaml, sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" ci/gitea.yml ci/github.yml && echo "CI YAMLs: ok"
```

Expected: all three "ok".

- [ ] **Step 6: Commit**

```bash
git add ci/ install.sh upgrade.sh
git commit -m "feat: add CI templates and install/upgrade scripts"
```

---

## Task 10: GitHub repo + publish

**Files:** None (git/GitHub operations only)

- [x] **Step 1: Create GitHub repo**

```bash
gh repo create 89jobrien/devkit --private --description "AI-powered dev workflow toolkit for any project" --source=. --remote=origin
```

- [x] **Step 2: Push**

```bash
git push -u origin main
```

- [x] **Step 3: Create v1.0.0 tag**

```bash
git tag v1.0.0
git push origin v1.0.0
```

- [x] **Step 4: Verify `go install` works**

```bash
go install github.com/89jobrien/devkit/cmd/devkit@v1.0.0
devkit --help
```

Expected: help output with council/review/meta subcommands.

- [x] **Step 5: Verify `go run ci-agent` resolves**

```bash
go run github.com/89jobrien/devkit/cmd/ci-agent@v1.0.0 2>&1 | grep -i "required env var"
```

Expected: error about missing `CI_PLATFORM` env var (confirming it runs).

- [x] **Step 6: Final test run**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 7: Commit anything remaining + push**

```bash
git status
git push
```
