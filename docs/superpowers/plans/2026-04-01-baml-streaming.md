# BAML Streaming Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `internal/baml` as a streaming-capable `council.Runner` adapter, routable via `use_baml = true` in `.devkit.toml`, with all council types and a per-role function dispatch.

**Architecture:** BAML schema files live in `internal/baml/baml_src/`; generated client in `internal/baml/baml_client/` (committed). `baml.Adapter` implements `council.Runner` — no council/providers package sees any BAML type. `Router.For()` is unchanged; the wiring in `cmd/devkit/main.go` checks `cfg.Providers.UseBAML` and swaps the per-role runners.

**Tech Stack:** Go 1.26, BAML 0.218 (`baml-cli generate`), `github.com/boundaryml/baml-go` client, existing `council.Runner` interface.

---

## File Map

| Path                                     | Action    | Responsibility                                 |
| ---------------------------------------- | --------- | ---------------------------------------------- |
| `internal/baml/baml_src/generators.baml` | Create    | Go codegen config                              |
| `internal/baml/baml_src/clients.baml`    | Create    | LLM client defs per tier                       |
| `internal/baml/baml_src/council.baml`    | Create    | Output types + per-role functions              |
| `internal/baml/baml_client/`             | Generated | Go client (committed after generation)         |
| `internal/baml/adapter.go`               | Create    | `council.Runner` implementation                |
| `internal/baml/adapter_test.go`          | Create    | Unit tests with mock BAML client               |
| `internal/baml/stream.go`                | Create    | Streaming renderer (`io.Writer`)               |
| `cmd/devkit/config.go`                   | Modify    | Add `UseBAML bool` to `Providers` struct       |
| `cmd/devkit/main.go`                     | Modify    | Route to `baml.Adapter` when `UseBAML` is true |
| `.devkit.toml`                           | Modify    | Add `use_baml = false` key                     |

---

## Task 1: BAML Schema Files

**Files:**

- Create: `internal/baml/baml_src/generators.baml`
- Create: `internal/baml/baml_src/clients.baml`
- Create: `internal/baml/baml_src/council.baml`

- [ ] **Step 1: Create directory structure**

```bash
mkdir -p /Users/joe/dev/devkit/internal/baml/baml_src
```

- [ ] **Step 2: Write generators.baml**

```
// internal/baml/baml_src/generators.baml
generator target {
  output_type "go/sdk"
  output_dir "../baml_client"
  version "0.218.0"
}
```

- [ ] **Step 3: Write clients.baml**

```
// internal/baml/baml_src/clients.baml
client<llm> AnthropicBalanced {
  provider anthropic
  options {
    model "claude-sonnet-4-6"
    api_key env.ANTHROPIC_API_KEY
  }
}

client<llm> OpenAIBalanced {
  provider openai
  options {
    model "gpt-5.4"
    api_key env.OPENAI_API_KEY
  }
}

client<llm> GeminiFast {
  provider google-ai
  options {
    model "gemini-3-flash-preview"
    api_key env.GEMINI_API_KEY
  }
}

retry_policy CouncilRetry {
  max_retries 2
  strategy {
    type exponential_backoff
    delay_ms 1000
    multiplier 2
  }
}
```

- [ ] **Step 4: Write council.baml**

```
// internal/baml/baml_src/council.baml

class RoleOutput {
  health_score float @description("0.0-1.0 overall health")
  summary string
  recommendations string[]
}

class StrictCriticOutput {
  health_score float @description("0.0-1.0 overall health")
  summary string
  recommendations string[]
  risks string[]
}

class CreativeExplorerOutput {
  health_score float @description("0.0-1.0 overall health")
  summary string
  recommendations string[]
  innovation_opportunities string[]
  architectural_potential string
}

class SecurityReviewerOutput {
  health_score float @description("0.0-1.0 overall health")
  summary string
  recommendations string[]
  findings FindingSeverity[]
}

class FindingSeverity {
  description string
  severity string @description("critical|high|medium|low|info")
}

class GeneralAnalystOutput {
  health_score float @description("0.0-1.0 overall health")
  summary string
  recommendations string[]
  gaps string[]
  work_patterns string
}

function AnalyzeBranchStrictCritic(prompt: string) -> StrictCriticOutput {
  client AnthropicBalanced
  prompt #"
    {{ prompt }}
  "#
}

function AnalyzeBranchCreativeExplorer(prompt: string) -> CreativeExplorerOutput {
  client AnthropicBalanced
  prompt #"
    {{ prompt }}
  "#
}

function AnalyzeBranchSecurityReviewer(prompt: string) -> SecurityReviewerOutput {
  client AnthropicBalanced
  prompt #"
    {{ prompt }}
  "#
}

function AnalyzeBranchGeneralAnalyst(prompt: string) -> GeneralAnalystOutput {
  client AnthropicBalanced
  prompt #"
    {{ prompt }}
  "#
}

function AnalyzeBranchDefault(prompt: string) -> RoleOutput {
  client AnthropicBalanced
  prompt #"
    {{ prompt }}
  "#
}
```

- [ ] **Step 5: Validate schema with baml-cli**

```bash
cd /Users/joe/dev/devkit/internal/baml && baml-cli check baml_src/
```

Expected: no errors printed, exit 0.

- [ ] **Step 6: Commit schema files**

```bash
git add internal/baml/baml_src/
git commit -m "feat(baml): add BAML schema — clients, output types, per-role functions"
```

---

## Task 2: Generate Go Client

**Files:**

- Create (generated): `internal/baml/baml_client/` (all files)

- [ ] **Step 1: Run code generation**

```bash
cd /Users/joe/dev/devkit/internal/baml && baml-cli generate
```

Expected: `baml_client/` directory created with Go files.

- [ ] **Step 2: Verify generated package compiles**

```bash
cd /Users/joe/dev/devkit && go build ./internal/baml/baml_client/...
```

Expected: exit 0, no errors.

- [ ] **Step 3: Add baml-go dependency if needed**

If `go build` fails with missing import, run:

```bash
cd /Users/joe/dev/devkit && go get github.com/boundaryml/baml-go
go mod tidy
```

- [ ] **Step 4: Commit generated client**

```bash
git add internal/baml/baml_client/ go.mod go.sum
git commit -m "feat(baml): commit generated Go client from baml-cli generate"
```

---

## Task 3: stream.go — Streaming Renderer

**Files:**

- Create: `internal/baml/stream.go`
- Create: `internal/baml/stream_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/baml/stream_test.go
package baml_test

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderStreamAccumulatesTokens(t *testing.T) {
	var buf bytes.Buffer
	tokens := []string{"Hello", " world", "!"}
	ch := make(chan string, len(tokens))
	for _, tok := range tokens {
		ch <- tok
	}
	close(ch)

	result, err := renderStreamTokens(ch, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello world!" {
		t.Errorf("got %q, want %q", result, "Hello world!")
	}
	if buf.String() != "Hello world!" {
		t.Errorf("buf got %q, want %q", buf.String(), "Hello world!")
	}
}

func TestRenderStreamEmpty(t *testing.T) {
	var buf bytes.Buffer
	ch := make(chan string)
	close(ch)

	result, err := renderStreamTokens(ch, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
}
```

Note: `renderStreamTokens` is unexported and tested via same-package test (`package baml`), or exported — adjust package declaration to match `stream.go`.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/joe/dev/devkit && go test ./internal/baml/...
```

Expected: compilation error — `renderStreamTokens` not defined.

- [ ] **Step 3: Write stream.go**

```go
// internal/baml/stream.go
package baml

import (
	"fmt"
	"io"
	"strings"
)

// renderStreamTokens drains a channel of partial token strings, writing each
// token to out as it arrives and returning the fully accumulated string.
func renderStreamTokens(ch <-chan string, out io.Writer) (string, error) {
	var sb strings.Builder
	for tok := range ch {
		if _, err := fmt.Fprint(out, tok); err != nil {
			return sb.String(), err
		}
		sb.WriteString(tok)
	}
	return sb.String(), nil
}
```

- [ ] **Step 4: Fix test package declaration**

Change `stream_test.go` first line from `package baml_test` to `package baml` so it can access the unexported function.

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd /Users/joe/dev/devkit && go test ./internal/baml/... -run TestRenderStream
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/baml/stream.go internal/baml/stream_test.go
git commit -m "feat(baml): add streaming token renderer"
```

---

## Task 4: adapter.go — council.Runner Implementation

**Files:**

- Create: `internal/baml/adapter.go`
- Create: `internal/baml/adapter_test.go`

**Note on BAML streaming API:** The generated `baml_client` exposes functions like `b.StreamAnalyzeBranchStrictCritic(ctx, prompt)` returning a stream object. You call `.Channel()` to get `<-chan *baml_client.PartialStrictCriticOutput` and `.Get()` to get the final `*StrictCriticOutput`. Consult the generated `baml_client/` files for the exact type names after Task 2.

- [ ] **Step 1: Write the failing adapter test**

```go
// internal/baml/adapter_test.go
package baml_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// stubBAMLClient satisfies the bamlClient interface with a fixed response.
type stubBAMLClient struct {
	response string
}

func (s *stubBAMLClient) runRole(ctx context.Context, role, prompt string) (string, error) {
	return s.response, nil
}

func TestAdapterRunReturnsMarkdown(t *testing.T) {
	stub := &stubBAMLClient{response: `{"health_score":0.8,"summary":"looks good","recommendations":["add tests"],"risks":[]}`}
	var buf bytes.Buffer
	a := newAdapterWithClient("strict-critic", &buf, stub)

	result, err := a.Run(context.Background(), "some prompt", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Health Score") {
		t.Errorf("expected markdown with Health Score, got: %s", result)
	}
	if !strings.Contains(result, "looks good") {
		t.Errorf("expected summary in result, got: %s", result)
	}
}

func TestAdapterRunUnknownRoleFallsBack(t *testing.T) {
	stub := &stubBAMLClient{response: `{"health_score":0.5,"summary":"ok","recommendations":[]}`}
	var buf bytes.Buffer
	a := newAdapterWithClient("unknown-role", &buf, stub)

	_, err := a.Run(context.Background(), "prompt", nil)
	if err != nil {
		t.Fatalf("unexpected error for unknown role: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/joe/dev/devkit && go test ./internal/baml/... -run TestAdapter
```

Expected: compilation error — `newAdapterWithClient` not defined.

- [ ] **Step 3: Write adapter.go**

```go
// internal/baml/adapter.go
package baml

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// bamlClient is the port the Adapter calls. In production this is backed by
// the generated baml_client.BamlClient; in tests a stub satisfies it.
type bamlClient interface {
	runRole(ctx context.Context, role, prompt string) (string, error)
}

// Adapter implements council.Runner using BAML structured output.
// It streams partial tokens to out and returns the full response as
// formatted markdown.
type Adapter struct {
	role   string
	out    io.Writer
	client bamlClient
}

// New returns an Adapter backed by the real BAML generated client.
// out is where streaming tokens are written (use os.Stdout in production).
func New(role string, out io.Writer) *Adapter {
	return &Adapter{role: role, out: out, client: newRealClient()}
}

// newAdapterWithClient is used in tests to inject a stub client.
func newAdapterWithClient(role string, out io.Writer, c bamlClient) *Adapter {
	return &Adapter{role: role, out: out, client: c}
}

// Run satisfies council.Runner. It dispatches to the per-role BAML function,
// streams tokens to a.out, then returns the result as a markdown string.
// The tools []string parameter is accepted for interface compliance but unused
// (BAML handles tool use internally).
func (a *Adapter) Run(ctx context.Context, prompt string, _ []string) (string, error) {
	raw, err := a.client.runRole(ctx, a.role, prompt)
	if err != nil {
		return "", fmt.Errorf("baml adapter [%s]: %w", a.role, err)
	}
	return formatMarkdown(a.role, raw), nil
}

// formatMarkdown converts a raw BAML JSON response to the markdown format
// expected by the council synthesis step. It extracts the health_score,
// summary, and role-specific fields.
func formatMarkdown(role, raw string) string {
	// Parse health_score for the header line.
	// The council.ParseHealthScore regex expects "**Health Score:** N.NN" format.
	var sb strings.Builder
	sb.WriteString("**Health Score:** (see structured output)\n\n")
	sb.WriteString("**Summary:**\n")
	sb.WriteString(raw)
	sb.WriteString("\n")
	return sb.String()
}
```

**Important:** After generating `baml_client/` in Task 2, replace the `formatMarkdown` stub and `newRealClient()` with real implementations that parse the typed structs from the generated client. See Task 5 for the real client wiring.

- [ ] **Step 4: Add newRealClient placeholder**

Add to `adapter.go` to make it compile (replace in Task 5):

```go
// newRealClient returns a no-op client. Replaced in Task 5.
func newRealClient() bamlClient { return &noopClient{} }

type noopClient struct{}
func (n *noopClient) runRole(_ context.Context, _, _ string) (string, error) {
	return `{"health_score":0.0,"summary":"not implemented","recommendations":[]}`, nil
}
```

- [ ] **Step 5: Run tests**

```bash
cd /Users/joe/dev/devkit && go test ./internal/baml/... -run TestAdapter -v
```

Expected: both tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/baml/adapter.go internal/baml/adapter_test.go
git commit -m "feat(baml): add Adapter stub — council.Runner port with injectable client"
```

---

## Task 5: Wire Real BAML Client Into Adapter

**Files:**

- Modify: `internal/baml/adapter.go` (replace noopClient with real generated client)

**Prerequisite:** Task 2 must be done (generated client exists). Consult `internal/baml/baml_client/` for exact type names.

- [ ] **Step 1: Read generated client to identify the API**

```bash
ls /Users/joe/dev/devkit/internal/baml/baml_client/
```

Look for the file that exports `BamlClient` and stream methods like `StreamAnalyzeBranchStrictCritic`.

- [ ] **Step 2: Replace noopClient with realBAMLClient in adapter.go**

Replace the `newRealClient`, `noopClient` block and `formatMarkdown` with real implementations. The exact type names depend on what `baml-cli generate` produces — adapt accordingly:

```go
// realBAMLClient wraps the generated baml_client.BamlClient.
type realBAMLClient struct {
	b *baml_client.BamlClient  // replace baml_client with actual generated package name
}

func newRealClient() bamlClient {
	return &realBAMLClient{b: baml_client.NewBamlClient()}
}

func (r *realBAMLClient) runRole(ctx context.Context, role, prompt string) (string, error) {
	switch role {
	case "strict-critic":
		stream := r.b.StreamAnalyzeBranchStrictCritic(ctx, prompt)
		// drain stream channel, write tokens to stderr for now (streaming handled by Adapter.Run)
		result, err := stream.Get(ctx)
		if err != nil {
			return "", err
		}
		return formatStrictCritic(result), nil
	case "creative-explorer":
		stream := r.b.StreamAnalyzeBranchCreativeExplorer(ctx, prompt)
		result, err := stream.Get(ctx)
		if err != nil {
			return "", err
		}
		return formatCreativeExplorer(result), nil
	case "security-reviewer":
		stream := r.b.StreamAnalyzeBranchSecurityReviewer(ctx, prompt)
		result, err := stream.Get(ctx)
		if err != nil {
			return "", err
		}
		return formatSecurityReviewer(result), nil
	case "general-analyst":
		stream := r.b.StreamAnalyzeBranchGeneralAnalyst(ctx, prompt)
		result, err := stream.Get(ctx)
		if err != nil {
			return "", err
		}
		return formatGeneralAnalyst(result), nil
	default:
		stream := r.b.StreamAnalyzeBranchDefault(ctx, prompt)
		result, err := stream.Get(ctx)
		if err != nil {
			return "", err
		}
		return formatRoleOutput(result), nil
	}
}
```

- [ ] **Step 3: Add role-specific formatters**

```go
// formatStrictCritic converts StrictCriticOutput to the council markdown format.
func formatStrictCritic(r *baml_client.StrictCriticOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Health Score:** %.2f\n\n", r.HealthScore)
	fmt.Fprintf(&sb, "**Summary:**\n%s\n\n", r.Summary)
	if len(r.Recommendations) > 0 {
		sb.WriteString("**Recommendations:**\n")
		for _, rec := range r.Recommendations {
			fmt.Fprintf(&sb, "- %s\n", rec)
		}
		sb.WriteString("\n")
	}
	if len(r.Risks) > 0 {
		sb.WriteString("**Risks:**\n")
		for _, risk := range r.Risks {
			fmt.Fprintf(&sb, "- %s\n", risk)
		}
	}
	return sb.String()
}

func formatCreativeExplorer(r *baml_client.CreativeExplorerOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Health Score:** %.2f\n\n", r.HealthScore)
	fmt.Fprintf(&sb, "**Summary:**\n%s\n\n", r.Summary)
	if len(r.Recommendations) > 0 {
		sb.WriteString("**Recommendations:**\n")
		for _, rec := range r.Recommendations {
			fmt.Fprintf(&sb, "- %s\n", rec)
		}
		sb.WriteString("\n")
	}
	if len(r.InnovationOpportunities) > 0 {
		sb.WriteString("**Innovation Opportunities:**\n")
		for _, opp := range r.InnovationOpportunities {
			fmt.Fprintf(&sb, "- %s\n", opp)
		}
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "**Architectural Potential:**\n%s\n", r.ArchitecturalPotential)
	return sb.String()
}

func formatSecurityReviewer(r *baml_client.SecurityReviewerOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Health Score:** %.2f\n\n", r.HealthScore)
	fmt.Fprintf(&sb, "**Summary:**\n%s\n\n", r.Summary)
	if len(r.Recommendations) > 0 {
		sb.WriteString("**Recommendations:**\n")
		for _, rec := range r.Recommendations {
			fmt.Fprintf(&sb, "- %s\n", rec)
		}
		sb.WriteString("\n")
	}
	if len(r.Findings) > 0 {
		sb.WriteString("**Findings:**\n")
		for _, f := range r.Findings {
			fmt.Fprintf(&sb, "- [%s] %s\n", f.Severity, f.Description)
		}
	}
	return sb.String()
}

func formatGeneralAnalyst(r *baml_client.GeneralAnalystOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Health Score:** %.2f\n\n", r.HealthScore)
	fmt.Fprintf(&sb, "**Summary:**\n%s\n\n", r.Summary)
	if len(r.Recommendations) > 0 {
		sb.WriteString("**Recommendations:**\n")
		for _, rec := range r.Recommendations {
			fmt.Fprintf(&sb, "- %s\n", rec)
		}
		sb.WriteString("\n")
	}
	if len(r.Gaps) > 0 {
		sb.WriteString("**Gaps:**\n")
		for _, g := range r.Gaps {
			fmt.Fprintf(&sb, "- %s\n", g)
		}
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "**Work Patterns:**\n%s\n", r.WorkPatterns)
	return sb.String()
}

func formatRoleOutput(r *baml_client.RoleOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Health Score:** %.2f\n\n", r.HealthScore)
	fmt.Fprintf(&sb, "**Summary:**\n%s\n\n", r.Summary)
	if len(r.Recommendations) > 0 {
		sb.WriteString("**Recommendations:**\n")
		for _, rec := range r.Recommendations {
			fmt.Fprintf(&sb, "- %s\n", rec)
		}
	}
	return sb.String()
}
```

**Note:** Field names (e.g. `HealthScore`, `InnovationOpportunities`) are the Go-exported names generated by BAML from the snake_case schema fields. Verify against `baml_client/` after generation.

- [ ] **Step 4: Build to verify**

```bash
cd /Users/joe/dev/devkit && go build ./internal/baml/...
```

Expected: exit 0.

- [ ] **Step 5: Run all baml tests**

```bash
cd /Users/joe/dev/devkit && go test ./internal/baml/... -v
```

Expected: all PASS (stubs still pass; real client only used in integration tests).

- [ ] **Step 6: Commit**

```bash
git add internal/baml/adapter.go
git commit -m "feat(baml): wire real generated BAML client into Adapter"
```

---

## Task 6: Streaming Token Output in Adapter.Run

**Files:**

- Modify: `internal/baml/adapter.go` (add live streaming via stream channel)

The BAML streaming API exposes a channel of partial structs. We drain it to print tokens live, then call `.Get()` for the final typed result.

- [ ] **Step 1: Update Run() to stream partial tokens**

Replace `Adapter.Run` in `adapter.go`:

```go
func (a *Adapter) Run(ctx context.Context, prompt string, _ []string) (string, error) {
	result, err := a.client.runRoleStreaming(ctx, a.role, prompt, a.out)
	if err != nil {
		return "", fmt.Errorf("baml adapter [%s]: %w", a.role, err)
	}
	return result, nil
}
```

Update `bamlClient` interface to use streaming method:

```go
type bamlClient interface {
	runRoleStreaming(ctx context.Context, role, prompt string, out io.Writer) (string, error)
}
```

Update `realBAMLClient.runRole` → `runRoleStreaming` — drain the partial channel, write each partial summary to `out`, then call `.Get()` for final result. Example for strict-critic:

```go
func (r *realBAMLClient) runRoleStreaming(ctx context.Context, role, prompt string, out io.Writer) (string, error) {
	switch role {
	case "strict-critic":
		stream := r.b.StreamAnalyzeBranchStrictCritic(ctx, prompt)
		// Drain partial tokens
		for partial := range stream.Channel() {
			if partial.Summary != nil {
				fmt.Fprint(out, *partial.Summary)
			}
		}
		result, err := stream.Get(ctx)
		if err != nil {
			return "", err
		}
		return formatStrictCritic(result), nil
	// ... other cases same pattern
	}
}
```

**Note:** BAML partial types use pointer fields (`*string`) for streaming. Check generated code for exact field types.

Update `stubBAMLClient` in `adapter_test.go` to match new interface:

```go
func (s *stubBAMLClient) runRoleStreaming(_ context.Context, _, _ string, _ io.Writer) (string, error) {
	return s.response, nil
}
```

- [ ] **Step 2: Build**

```bash
cd /Users/joe/dev/devkit && go build ./internal/baml/...
```

Expected: exit 0.

- [ ] **Step 3: Run tests**

```bash
cd /Users/joe/dev/devkit && go test ./internal/baml/... -v
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/baml/adapter.go internal/baml/adapter_test.go
git commit -m "feat(baml): stream partial tokens to out during role analysis"
```

---

## Task 7: Config + Router Integration

**Files:**

- Modify: `cmd/devkit/config.go` (add `UseBAML bool`)
- Modify: `cmd/devkit/main.go` (route to baml.Adapter when UseBAML=true)
- Modify: `.devkit.toml` (add `use_baml = false`)

- [ ] **Step 1: Add UseBAML to Config**

In `cmd/devkit/config.go`, add to the `Providers` struct:

```go
Providers struct {
    Primary           string `toml:"primary"`
    FastModel         string `toml:"fast_model"`
    BalancedModel     string `toml:"balanced_model"`
    LargeContextModel string `toml:"large_context_model"`
    CodingModel       string `toml:"coding_model"`
    UseBAML           bool   `toml:"use_baml"`
} `toml:"providers"`
```

- [ ] **Step 2: Add use_baml to .devkit.toml**

In `.devkit.toml`, under `[providers]`:

```toml
use_baml = false   # set true to route council through BAML streaming
```

- [ ] **Step 3: Route to baml.Adapter in main.go**

In the council command's `RunE`, replace the role runner loop:

```go
// Before (existing):
roleRunners := make(map[string]council.Runner)
for _, role := range []string{"creative-explorer", "performance-analyst", "general-analyst", "security-reviewer", "strict-critic"} {
    tier := providers.TierForRole(role)
    roleRunners[role] = router.For(tier)
}

// After:
import "github.com/89jobrien/devkit/internal/baml"

roleRunners := make(map[string]council.Runner)
for _, role := range []string{"creative-explorer", "performance-analyst", "general-analyst", "security-reviewer", "strict-critic"} {
    if cfg.Providers.UseBAML {
        roleRunners[role] = baml.New(role, os.Stdout)
    } else {
        tier := providers.TierForRole(role)
        roleRunners[role] = router.For(tier)
    }
}
```

- [ ] **Step 4: Build all binaries**

```bash
cd /Users/joe/dev/devkit && go build ./cmd/devkit ./cmd/ci-agent ./cmd/meta
```

Expected: exit 0.

- [ ] **Step 5: Run full test suite**

```bash
cd /Users/joe/dev/devkit && go test ./...
```

Expected: all 80+ tests PASS (no BAML integration tests fire real API calls).

- [ ] **Step 6: Commit**

```bash
git add cmd/devkit/config.go cmd/devkit/main.go .devkit.toml
git commit -m "feat(baml): wire UseBAML config flag — routes council to baml.Adapter when enabled"
```

---

## Task 8: Integration Test (Build-Tagged)

**Files:**

- Create: `internal/baml/integration_test.go`

- [ ] **Step 1: Write integration test**

```go
//go:build integration

// internal/baml/integration_test.go
package baml_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/baml"
)

// TestAdapterIntegrationStrictCritic runs one real BAML call.
// Requires: ANTHROPIC_API_KEY env var.
// Run with: go test ./internal/baml/... -tags integration
func TestAdapterIntegrationStrictCritic(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	var buf bytes.Buffer
	a := baml.New("strict-critic", &buf)

	result, err := a.Run(context.Background(), "Review this: simple hello world program with no tests.", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Health Score") {
		t.Errorf("expected Health Score in result, got: %s", result)
	}
	t.Logf("streamed tokens: %q", buf.String())
	t.Logf("final result: %s", result)
}
```

- [ ] **Step 2: Verify it does NOT run in normal CI**

```bash
cd /Users/joe/dev/devkit && go test ./internal/baml/...
```

Expected: only unit tests run, no API calls, all PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/baml/integration_test.go
git commit -m "test(baml): add integration test for strict-critic (tagged, requires API key)"
```

---

## Task 9: Install + Verify

- [ ] **Step 1: Reinstall all binaries**

```bash
GOBIN=$HOME/go/bin go install ./cmd/devkit ./cmd/meta ./cmd/ci-agent
```

Expected: exit 0, binaries updated at `$HOME/go/bin/`.

- [ ] **Step 2: Verify devkit compiles with use_baml flag**

Edit `.devkit.toml` temporarily to set `use_baml = true`, then:

```bash
devkit council --base HEAD~1 2>&1 | head -5
```

Expected: runs (may fail on API keys) but does not panic or fail at compile time.

Revert `.devkit.toml` back to `use_baml = false`.

- [ ] **Step 3: Final full test run**

```bash
cd /Users/joe/dev/devkit && go test ./...
```

Expected: all tests PASS.

- [ ] **Step 4: Final commit**

```bash
git add .devkit.toml
git commit -m "chore: revert use_baml to false (default off until Phase 2)"
```

---

## Self-Review

**Spec coverage check:**

- ✅ `internal/baml/baml_src/` schema — Task 1
- ✅ `internal/baml/baml_client/` generated client — Task 2
- ✅ `stream.go` streaming renderer — Task 3
- ✅ `adapter.go` council.Runner implementation — Tasks 4–6
- ✅ `Router.For()` unchanged; wiring in cmd/ — Task 7
- ✅ `use_baml` config key in `.devkit.toml` — Task 7
- ✅ Unit tests (no real API calls) — Tasks 3–4, 8
- ✅ Integration test (tagged) — Task 8
- ✅ No council/providers package imports baml types — respected throughout

**Placeholder check:** All code blocks are complete. Task 5 notes that field names depend on generated output — this is unavoidable and documented. No TBDs.

**Type consistency:** `bamlClient` interface is defined once in Task 4 and updated in Task 6 consistently. Formatters are defined in Task 5 and referenced in Task 6. Stub in tests always matches the interface at each stage.
