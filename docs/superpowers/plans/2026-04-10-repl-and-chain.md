# REPL and Chain Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a persistent REPL with session context accumulation and a fixed-order agent
chain pipeline, both terminating with a gpt-5.4 synthesis agent.

**Architecture:** `internal/chain/` owns the `Result` envelope, stage registry, preflight
validator, pipeline runner, and synthesis agent. `internal/repl/` owns the readline session
loop and session context store. `cmd/devkit/cmd_chain.go` and `cmd_repl.go` wire cobra
commands. The pipeline always runs: `[preflight] → selected stages in fixed order →
[synthesis]`. The REPL persists auth and accumulates `[]Result` across commands; `--no-context`
opts out per-command.

**Tech Stack:** Go 1.26, `github.com/spf13/cobra`, `github.com/chzyer/readline` (add to
go.mod), OpenAI provider via existing `internal/ai/providers` for synthesis (model: `gpt-5.4`).

---

## File Map

| File | Create/Modify | Responsibility |
|------|--------------|----------------|
| `internal/chain/result.go` | Create | `Result` envelope + typed payload structs |
| `internal/chain/stage.go` | Create | `Stage` interface, fixed registry, ordering |
| `internal/chain/preflight.go` | Create | Preflight validator (env, stage names, binaries) |
| `internal/chain/pipeline.go` | Create | Pipeline runner: preflight → stages → synthesis |
| `internal/chain/synthesis.go` | Create | gpt-5.4 synthesis agent, always-last stage |
| `internal/chain/pipeline_test.go` | Create | Pipeline unit tests with stub stages |
| `internal/chain/preflight_test.go` | Create | Preflight unit tests |
| `internal/repl/session.go` | Create | Session context store, `--no-context` handling |
| `internal/repl/repl.go` | Create | Readline loop, command dispatch, auth persistence |
| `internal/repl/repl_test.go` | Create | Session accumulation tests |
| `cmd/devkit/cmd_chain.go` | Create | `devkit chain <stages...>` cobra command |
| `cmd/devkit/cmd_repl.go` | Create | `devkit repl` cobra command |
| `cmd/devkit/main.go` | Modify | Register `chain` and `repl` commands |
| `go.mod` / `go.sum` | Modify | Add `github.com/chzyer/readline` |

---

## Task 1: Result envelope and typed payloads

**Files:**
- Create: `internal/chain/result.go`
- Create: `internal/chain/result_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/chain/result_test.go
package chain_test

import (
    "errors"
    "testing"

    "github.com/89jobrien/devkit/internal/chain"
    "github.com/stretchr/testify/assert"
)

func TestResultIsSkipped(t *testing.T) {
    r := chain.Result{}
    assert.True(t, r.IsSkipped())
}

func TestResultNotSkippedWhenOutput(t *testing.T) {
    r := chain.Result{Stage: "council", Output: "some output"}
    assert.False(t, r.IsSkipped())
}

func TestResultNotSkippedWhenError(t *testing.T) {
    r := chain.Result{Stage: "council", Err: errors.New("failed")}
    assert.False(t, r.IsSkipped())
}

func TestCouncilPayload(t *testing.T) {
    p := &chain.CouncilPayload{HealthScore: 0.87, RoleOutputs: map[string]string{"strict-critic": "text"}}
    r := chain.Result{Stage: "council", Output: "output", Payload: p}
    got, ok := r.Payload.(*chain.CouncilPayload)
    assert.True(t, ok)
    assert.InDelta(t, 0.87, got.HealthScore, 0.001)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/chain/... 2>&1 | head -10
```
Expected: `cannot find package` or `undefined: chain`

- [ ] **Step 3: Implement `result.go`**

```go
// internal/chain/result.go
package chain

// Result is the universal envelope for a single pipeline stage output.
// Skipped stages have zero-value Result with Stage == "".
// Payload carries the stage-specific typed struct; use a type switch to inspect it.
// Meta holds lightweight k/v for fields that don't warrant a dedicated struct.
type Result struct {
    Stage   string         // stage name, e.g. "council"
    Output  string         // rendered string output (always populated if not skipped)
    Payload any            // typed: *CouncilPayload, *CITriagePayload, etc.
    Err     error          // non-nil if stage failed; Output may still be partially populated
    Meta    map[string]any // lightweight k/v for unstructured metadata
}

// IsSkipped returns true when this slot represents a stage not selected by the user.
func (r Result) IsSkipped() bool {
    return r.Stage == "" && r.Output == "" && r.Err == nil
}

// --- Typed payload structs ---

// CouncilPayload carries structured output from the council stage.
type CouncilPayload struct {
    HealthScore float64           // meta-score average across roles (0–1)
    RoleOutputs map[string]string // role name → full role output text
}

// CITriagePayload carries structured output from the ci-triage stage.
type CITriagePayload struct {
    RootCause   string
    Suggestions []string
    LogSnippet  string // the filtered log sent to the runner
}

// LogPatternPayload carries structured output from the log-pattern stage.
type LogPatternPayload struct {
    Patterns []string // recurring error patterns found
    Count    int
}

// DiagnosePayload carries structured output from the diagnose stage.
type DiagnosePayload struct {
    Summary     string
    Severity    string // "low" | "medium" | "high" | "critical"
    NextActions []string
}

// TicketPayload carries structured output from the ticket stage.
type TicketPayload struct {
    Title  string
    Body   string
    Labels []string
}

// PRPayload carries structured output from the pr stage.
type PRPayload struct {
    Title string
    Body  string
}

// SynthesisPayload carries the final gpt-5.4 synthesis output.
type SynthesisPayload struct {
    Summary     string
    KeyFindings []string
    NextActions []string
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/chain/... -v -run TestResult 2>&1
```
Expected: all 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/chain/result.go internal/chain/result_test.go
git commit -m "feat(chain): add Result envelope and typed payload structs"
```

---

## Task 2: Stage interface and fixed registry

**Files:**
- Create: `internal/chain/stage.go`
- Create: `internal/chain/stage_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/chain/stage_test.go
package chain_test

import (
    "testing"

    "github.com/89jobrien/devkit/internal/chain"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestFixedOrderIsStable(t *testing.T) {
    // The canonical order must never change between calls.
    a := chain.CanonicalOrder()
    b := chain.CanonicalOrder()
    assert.Equal(t, a, b)
}

func TestCanonicalOrderContainsKnownStages(t *testing.T) {
    order := chain.CanonicalOrder()
    for _, name := range []string{"council", "ci-triage", "log-pattern", "diagnose", "ticket", "pr", "meta"} {
        assert.Contains(t, order, name, "canonical order missing stage %q", name)
    }
}

func TestSelectStages(t *testing.T) {
    slots, err := chain.SelectStages([]string{"council", "ticket"})
    require.NoError(t, err)
    // council is index 0, ticket is index 4 in canonical order
    assert.Equal(t, "council", slots[0].Name)
    assert.True(t, slots[0].Selected)
    assert.False(t, slots[1].Selected) // ci-triage skipped
    assert.Equal(t, "ticket", slots[4].Name)
    assert.True(t, slots[4].Selected)
}

func TestSelectStages_UnknownName(t *testing.T) {
    _, err := chain.SelectStages([]string{"council", "nonexistent"})
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "nonexistent")
}

func TestSelectStages_Empty(t *testing.T) {
    _, err := chain.SelectStages([]string{})
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "at least one stage")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/chain/... -run TestFixed 2>&1 | head -5
```
Expected: `undefined: chain.CanonicalOrder`

- [ ] **Step 3: Implement `stage.go`**

```go
// internal/chain/stage.go
package chain

import (
    "context"
    "fmt"
    "strings"
)

// StageRunner is the port for executing a single pipeline stage.
// Implementations call the underlying devkit command logic directly.
type StageRunner interface {
    Run(ctx context.Context, prior []Result) Result
}

// StageRunnerFunc adapts a function to StageRunner.
type StageRunnerFunc func(ctx context.Context, prior []Result) Result

func (f StageRunnerFunc) Run(ctx context.Context, prior []Result) Result {
    return f(ctx, prior)
}

// StageSlot represents one position in the fixed pipeline.
type StageSlot struct {
    Name     string
    Selected bool
    Runner   StageRunner // nil if not yet wired (set by cmd layer)
}

// canonicalOrder is the fixed execution order. Never reorder this slice.
var canonicalOrder = []string{
    "council",
    "ci-triage",
    "log-pattern",
    "diagnose",
    "ticket",
    "pr",
    "meta",
}

// CanonicalOrder returns the fixed stage execution order.
func CanonicalOrder() []string {
    out := make([]string, len(canonicalOrder))
    copy(out, canonicalOrder)
    return out
}

// SelectStages validates names and returns the full slot list in canonical order,
// with Selected=true for requested stages and Selected=false for skipped stages.
// Returns an error if any name is unknown or the list is empty.
func SelectStages(names []string) ([]StageSlot, error) {
    if len(names) == 0 {
        return nil, fmt.Errorf("chain: at least one stage required")
    }
    nameSet := make(map[string]bool, len(names))
    for _, n := range names {
        nameSet[strings.TrimSpace(n)] = true
    }
    // Validate all names against the canonical list.
    known := make(map[string]bool, len(canonicalOrder))
    for _, n := range canonicalOrder {
        known[n] = true
    }
    for n := range nameSet {
        if !known[n] {
            return nil, fmt.Errorf("chain: unknown stage %q (valid: %s)",
                n, strings.Join(canonicalOrder, ", "))
        }
    }
    slots := make([]StageSlot, len(canonicalOrder))
    for i, n := range canonicalOrder {
        slots[i] = StageSlot{Name: n, Selected: nameSet[n]}
    }
    return slots, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/chain/... -v -run "TestFixed|TestSelect|TestCanonical" 2>&1
```
Expected: all 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/chain/stage.go internal/chain/stage_test.go
git commit -m "feat(chain): add Stage interface, canonical order registry, SelectStages"
```

---

## Task 3: Preflight validator

**Files:**
- Create: `internal/chain/preflight.go`
- Create: `internal/chain/preflight_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/chain/preflight_test.go
package chain_test

import (
    "testing"

    "github.com/89jobrien/devkit/internal/chain"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestPreflightPassesWithValidConfig(t *testing.T) {
    cfg := chain.PreflightConfig{
        Stages:       []string{"council"},
        AnthropicKey: "ant-key",
        OpenAIKey:    "oai-key",
        RepoPath:     t.TempDir(),
        LookupBinary: func(name string) bool { return true },
    }
    errs := chain.Preflight(cfg)
    assert.Empty(t, errs)
}

func TestPreflightReportsAllFailures(t *testing.T) {
    cfg := chain.PreflightConfig{
        Stages:       []string{"council", "badstage"},
        AnthropicKey: "",
        OpenAIKey:    "",
        RepoPath:     "/nonexistent/path/xyz",
        LookupBinary: func(name string) bool { return false },
    }
    errs := chain.Preflight(cfg)
    // Expect: unknown stage, missing keys, bad repo path — reported all at once.
    assert.Greater(t, len(errs), 1, "expected multiple errors reported at once")
    msgs := make([]string, len(errs))
    for i, e := range errs {
        msgs[i] = e.Error()
    }
    combined := ""
    for _, m := range msgs {
        combined += m + "\n"
    }
    assert.Contains(t, combined, "badstage")
    assert.Contains(t, combined, "API key")
}

func TestPreflightRequiresGhForCITriage(t *testing.T) {
    ghPresent := false
    cfg := chain.PreflightConfig{
        Stages:       []string{"ci-triage"},
        AnthropicKey: "key",
        OpenAIKey:    "key",
        RepoPath:     t.TempDir(),
        LookupBinary: func(name string) bool {
            if name == "gh" {
                return ghPresent
            }
            return true
        },
    }
    errs := chain.Preflight(cfg)
    assert.Len(t, errs, 1)
    assert.Contains(t, errs[0].Error(), "gh")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/chain/... -run TestPreflight 2>&1 | head -5
```
Expected: `undefined: chain.Preflight`

- [ ] **Step 3: Implement `preflight.go`**

```go
// internal/chain/preflight.go
package chain

import (
    "fmt"
    "os"
)

// PreflightConfig holds everything Preflight needs to validate.
type PreflightConfig struct {
    Stages       []string          // user-requested stage names
    AnthropicKey string
    OpenAIKey    string
    RepoPath     string
    // LookupBinary checks whether a binary is on PATH. Overridable in tests.
    LookupBinary func(name string) bool
}

// binaryRequirements maps stage names to required binaries.
var binaryRequirements = map[string][]string{
    "ci-triage": {"gh"},
}

// Preflight validates env, stage names, repo path, and binary requirements.
// Returns all failures at once — never stops at the first error.
func Preflight(cfg PreflightConfig) []error {
    var errs []error

    // Validate stage names via SelectStages.
    if _, err := SelectStages(cfg.Stages); err != nil {
        errs = append(errs, err)
    }

    // At least one LLM key must be present (synthesis always needs OpenAI).
    if cfg.AnthropicKey == "" && cfg.OpenAIKey == "" {
        errs = append(errs, fmt.Errorf("preflight: at least one API key required (ANTHROPIC_API_KEY or OPENAI_API_KEY)"))
    }
    // Synthesis always uses OpenAI gpt-5.4.
    if cfg.OpenAIKey == "" {
        errs = append(errs, fmt.Errorf("preflight: OPENAI_API_KEY required for synthesis stage"))
    }

    // Repo path must exist if provided.
    if cfg.RepoPath != "" {
        if _, err := os.Stat(cfg.RepoPath); err != nil {
            errs = append(errs, fmt.Errorf("preflight: repo path %q not found: %w", cfg.RepoPath, err))
        }
    }

    // Binary requirements per stage.
    lookup := cfg.LookupBinary
    if lookup == nil {
        lookup = defaultLookup
    }
    for _, stage := range cfg.Stages {
        for _, bin := range binaryRequirements[stage] {
            if !lookup(bin) {
                errs = append(errs, fmt.Errorf("preflight: stage %q requires %q on PATH", stage, bin))
            }
        }
    }

    return errs
}

func defaultLookup(name string) bool {
    // exec.LookPath would import os/exec; use PATH search via os.Getenv instead.
    // This is intentionally simple — tests override via LookupBinary.
    path := os.Getenv("PATH")
    if path == "" {
        return false
    }
    // Delegate to exec.LookPath indirectly.
    _, err := lookPath(name)
    return err == nil
}
```

Add `lookPath` shim (avoids import cycle and keeps the func testable):

```go
// internal/chain/lookpath.go
package chain

import "os/exec"

// lookPath wraps exec.LookPath so defaultLookup can be replaced in tests
// without importing os/exec throughout the package.
var lookPath = exec.LookPath
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/chain/... -v -run TestPreflight 2>&1
```
Expected: all 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/chain/preflight.go internal/chain/preflight_test.go internal/chain/lookpath.go
git commit -m "feat(chain): add Preflight validator — reports all failures at once"
```

---

## Task 4: Synthesis stage (gpt-5.4, always last)

**Files:**
- Create: `internal/chain/synthesis.go`
- Create: `internal/chain/synthesis_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/chain/synthesis_test.go
package chain_test

import (
    "context"
    "net/http"
    "net/http/httptest"
    "encoding/json"
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/chain/... -run TestSynthesis 2>&1 | head -5
```
Expected: `undefined: chain.NewSynthesisRunner`

- [ ] **Step 3: Implement `synthesis.go`**

```go
// internal/chain/synthesis.go
package chain

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
)

const synthesisModel = "gpt-5.4"

// SynthesisRunner is the always-last pipeline stage powered by OpenAI gpt-5.4.
type SynthesisRunner struct {
    apiKey  string
    baseURL string // overridable for tests; empty means production
}

// NewSynthesisRunner constructs a SynthesisRunner. Pass a non-empty baseURL to
// redirect requests to an httptest server in tests.
func NewSynthesisRunner(apiKey, baseURL string) *SynthesisRunner {
    if baseURL == "" {
        baseURL = "https://api.openai.com"
    }
    return &SynthesisRunner{apiKey: apiKey, baseURL: baseURL}
}

// Run synthesizes all prior stage results into a final coherent summary.
// It always runs, even if prior stages errored — partial context is better than none.
func (s *SynthesisRunner) Run(ctx context.Context, prior []Result) Result {
    prompt := buildSynthesisPrompt(prior)
    output, err := s.callOpenAI(ctx, prompt)
    if err != nil {
        return Result{Stage: "synthesis", Err: fmt.Errorf("synthesis: %w", err)}
    }
    return Result{
        Stage:   "synthesis",
        Output:  output,
        Payload: &SynthesisPayload{Summary: output},
    }
}

func buildSynthesisPrompt(prior []Result) string {
    var sb strings.Builder
    sb.WriteString("You are a senior engineering lead synthesizing the output of a multi-stage\n")
    sb.WriteString("automated analysis pipeline. Produce a concise, actionable summary covering:\n")
    sb.WriteString("key findings, root causes, and recommended next actions.\n\n")
    sb.WriteString("## Pipeline Results\n\n")
    for _, r := range prior {
        if r.IsSkipped() {
            sb.WriteString("- [skipped]\n")
            continue
        }
        if r.Err != nil {
            fmt.Fprintf(&sb, "### %s (FAILED: %v)\n\n", r.Stage, r.Err)
            continue
        }
        fmt.Fprintf(&sb, "### %s\n\n%s\n\n", r.Stage, r.Output)
    }
    sb.WriteString("---\nProvide your synthesis now.")
    return sb.String()
}

func (s *SynthesisRunner) callOpenAI(ctx context.Context, prompt string) (string, error) {
    body, _ := json.Marshal(map[string]any{
        "model": synthesisModel,
        "messages": []map[string]any{
            {"role": "user", "content": prompt},
        },
        "max_completion_tokens": 2048,
    })
    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        s.baseURL+"/v1/chat/completions", bytes.NewReader(body))
    if err != nil {
        return "", err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+s.apiKey)

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        raw, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("openai %d: %s", resp.StatusCode, string(raw))
    }
    var out struct {
        Choices []struct {
            Message struct {
                Content string `json:"content"`
            } `json:"message"`
        } `json:"choices"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return "", fmt.Errorf("decode: %w", err)
    }
    if len(out.Choices) == 0 {
        return "", fmt.Errorf("no choices in response")
    }
    return out.Choices[0].Message.Content, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/chain/... -v -run TestSynthesis 2>&1
```
Expected: both tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/chain/synthesis.go internal/chain/synthesis_test.go
git commit -m "feat(chain): add SynthesisRunner — gpt-5.4, always-last stage"
```

---

## Task 5: Pipeline runner

**Files:**
- Create: `internal/chain/pipeline.go`
- Create: `internal/chain/pipeline_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/chain/pipeline_test.go
package chain_test

import (
    "context"
    "errors"
    "testing"

    "github.com/89jobrien/devkit/internal/chain"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func makeStubRunner(name, output string, err error) chain.StageRunnerFunc {
    return func(ctx context.Context, prior []chain.Result) chain.Result {
        return chain.Result{Stage: name, Output: output, Err: err}
    }
}

func TestPipelineRunsInOrder(t *testing.T) {
    var order []string
    makeOrdered := func(name string) chain.StageRunnerFunc {
        return func(ctx context.Context, prior []chain.Result) chain.Result {
            order = append(order, name)
            return chain.Result{Stage: name, Output: name + " output"}
        }
    }
    slots := []chain.StageSlot{
        {Name: "council", Selected: true, Runner: makeOrdered("council")},
        {Name: "ci-triage", Selected: false},
        {Name: "ticket", Selected: true, Runner: makeOrdered("ticket")},
    }
    synthesis := makeStubRunner("synthesis", "synth output", nil)
    results, err := chain.RunPipeline(context.Background(), slots, chain.StageRunnerFunc(synthesis))
    require.NoError(t, err)
    assert.Equal(t, []string{"council", "ticket"}, order)
    // results[0]=preflight, results[1]=council, results[2]=skipped, results[3]=ticket, results[N]=synthesis
    assert.Equal(t, "council", results[1].Stage)
    assert.True(t, results[2].IsSkipped())
    assert.Equal(t, "ticket", results[3].Stage)
    assert.Equal(t, "synthesis", results[len(results)-1].Stage)
}

func TestPipelineSkippedSlotIsNilResult(t *testing.T) {
    slots := []chain.StageSlot{
        {Name: "council", Selected: false},
        {Name: "ci-triage", Selected: true, Runner: makeStubRunner("ci-triage", "out", nil)},
    }
    synthesis := makeStubRunner("synthesis", "synth", nil)
    results, _ := chain.RunPipeline(context.Background(), slots, chain.StageRunnerFunc(synthesis))
    assert.True(t, results[1].IsSkipped(), "council slot should be skipped (zero Result)")
    assert.Equal(t, "ci-triage", results[2].Stage)
}

func TestPipelineContinuesAfterStageError(t *testing.T) {
    slots := []chain.StageSlot{
        {Name: "council", Selected: true, Runner: makeStubRunner("council", "", errors.New("council failed"))},
        {Name: "ci-triage", Selected: true, Runner: makeStubRunner("ci-triage", "ci output", nil)},
    }
    synthesis := makeStubRunner("synthesis", "synth", nil)
    results, err := chain.RunPipeline(context.Background(), slots, chain.StageRunnerFunc(synthesis))
    // Pipeline does not abort on stage error — synthesis always runs.
    require.NoError(t, err)
    assert.NotNil(t, results[1].Err)
    assert.Equal(t, "ci-triage", results[2].Stage)
    assert.Equal(t, "synthesis", results[len(results)-1].Stage)
}

func TestPipelineResultsLengthIsSlotsPlusTwoFixed(t *testing.T) {
    // Always: preflight(0) + len(slots) + synthesis(last)
    slots := []chain.StageSlot{
        {Name: "council", Selected: true, Runner: makeStubRunner("council", "out", nil)},
        {Name: "ci-triage", Selected: false},
    }
    synthesis := makeStubRunner("synthesis", "synth", nil)
    results, _ := chain.RunPipeline(context.Background(), slots, chain.StageRunnerFunc(synthesis))
    assert.Len(t, results, 1+len(slots)+1) // preflight + slots + synthesis
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/chain/... -run TestPipeline 2>&1 | head -5
```
Expected: `undefined: chain.RunPipeline`

- [ ] **Step 3: Implement `pipeline.go`**

```go
// internal/chain/pipeline.go
package chain

import "context"

// RunPipeline executes the pipeline:
//   results[0]         = preflight Result (Stage="preflight")
//   results[1..N]      = one slot per StageSlot (zero Result if not Selected)
//   results[N+1]       = synthesis Result (always runs)
//
// Errors from individual stages are captured in their Result.Err — the pipeline
// never aborts early. Synthesis always receives the full results slice.
func RunPipeline(ctx context.Context, slots []StageSlot, synthesis StageRunner) ([]Result, error) {
    results := make([]Result, 1+len(slots)+1)

    // Index 0: preflight (recorded as a no-op pass here; cmd layer runs real preflight before calling RunPipeline).
    results[0] = Result{Stage: "preflight", Output: "ok"}

    // Indices 1..N: stage slots.
    for i, slot := range slots {
        if !slot.Selected || slot.Runner == nil {
            // Leave as zero Result — IsSkipped() returns true.
            continue
        }
        prior := results[:i+1] // pass all results so far (read-only slice)
        results[i+1] = slot.Runner.Run(ctx, prior)
    }

    // Last index: synthesis always runs with all prior results.
    results[len(results)-1] = synthesis.Run(ctx, results[:len(results)-1])

    return results, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/chain/... -v -run TestPipeline 2>&1
```
Expected: all 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/chain/pipeline.go internal/chain/pipeline_test.go
git commit -m "feat(chain): add RunPipeline — preflight+stages+synthesis, never aborts"
```

---

## Task 6: Wire chain stages to existing devkit commands

**Files:**
- Create: `internal/chain/stages_wiring.go`
- Create: `internal/chain/stages_wiring_test.go`

This task wires `StageRunner` implementations for each stage using the existing
`internal/ops/*` and `internal/dev/*` packages. Each runner calls the package's `Run`
function directly and maps its output to the typed `Result`.

- [ ] **Step 1: Write the failing test**

```go
// internal/chain/stages_wiring_test.go
package chain_test

import (
    "context"
    "testing"

    "github.com/89jobrien/devkit/internal/chain"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestBuildCouncilRunner(t *testing.T) {
    // Stub council runner that always succeeds.
    stub := chain.StageRunnerFunc(func(ctx context.Context, prior []chain.Result) chain.Result {
        return chain.Result{Stage: "council", Output: "health: 0.9", Payload: &chain.CouncilPayload{HealthScore: 0.9}}
    })
    result := stub.Run(context.Background(), nil)
    require.NoError(t, result.Err)
    p, ok := result.Payload.(*chain.CouncilPayload)
    assert.True(t, ok)
    assert.InDelta(t, 0.9, p.HealthScore, 0.001)
}

func TestWiredStageSetsCorrectStageName(t *testing.T) {
    // Verify BuildStageRunners returns runners keyed to canonical stage names.
    cfg := chain.StageWiringConfig{
        RepoPath:     t.TempDir(),
        AnthropicKey: "key",
        OpenAIKey:    "key",
    }
    runners := chain.BuildStageRunners(cfg)
    for _, name := range chain.CanonicalOrder() {
        _, ok := runners[name]
        assert.True(t, ok, "missing runner for stage %q", name)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/chain/... -run "TestBuild|TestWired" 2>&1 | head -5
```
Expected: `undefined: chain.BuildStageRunners`

- [ ] **Step 3: Implement `stages_wiring.go`**

```go
// internal/chain/stages_wiring.go
package chain

import (
    "context"
    "fmt"

    "github.com/89jobrien/devkit/internal/ai/council"
    "github.com/89jobrien/devkit/internal/ai/providers"
    "github.com/89jobrien/devkit/internal/dev/pr"
    "github.com/89jobrien/devkit/internal/dev/ticket"
    "github.com/89jobrien/devkit/internal/ops/citriage"
    "github.com/89jobrien/devkit/internal/ops/diagnose"
    "github.com/89jobrien/devkit/internal/ops/logpattern"
    "github.com/89jobrien/devkit/internal/repocontext"
)

// StageWiringConfig holds the runtime config needed to construct stage runners.
type StageWiringConfig struct {
    RepoPath     string
    RunID        string // for ci-triage
    AnthropicKey string
    OpenAIKey    string
    GeminiKey    string
    ProviderURLs providers.RouterConfig // for test injection
}

// BuildStageRunners constructs a StageRunner for each canonical stage name.
// The returned map always has all canonical stage names as keys.
func BuildStageRunners(cfg StageWiringConfig) map[string]StageRunner {
    router := providers.NewRouter(providers.RouterConfig{
        AnthropicKey: cfg.AnthropicKey,
        OpenAIKey:    cfg.OpenAIKey,
        GeminiKey:    cfg.GeminiKey,
        AnthropicURL: cfg.ProviderURLs.AnthropicURL,
        OpenAIURL:    cfg.ProviderURLs.OpenAIURL,
        GeminiURL:    cfg.ProviderURLs.GeminiURL,
    })

    return map[string]StageRunner{
        "council": StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
            runner := router.For(providers.TierBalanced)
            base := resolveDiffBase(cfg.RepoPath)
            out, err := council.Run(ctx, council.Config{
                RepoPath: cfg.RepoPath,
                Base:     base,
                Runner:   runner,
            })
            if err != nil {
                return Result{Stage: "council", Err: fmt.Errorf("council: %w", err)}
            }
            return Result{Stage: "council", Output: out,
                Payload: &CouncilPayload{RoleOutputs: map[string]string{"synthesis": out}}}
        }),

        "ci-triage": StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
            runner := citriage.RunnerFunc(func(ctx context.Context, log, rc string) (string, error) {
                return router.For(providers.TierBalanced).Run(ctx, log+"\n"+rc, nil)
            })
            out, err := citriage.Run(ctx, citriage.Config{
                RepoPath: cfg.RepoPath,
                RunID:    cfg.RunID,
                Runner:   runner,
            })
            if err != nil {
                return Result{Stage: "ci-triage", Err: fmt.Errorf("ci-triage: %w", err)}
            }
            return Result{Stage: "ci-triage", Output: out,
                Payload: &CITriagePayload{RootCause: out}}
        }),

        "log-pattern": StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
            // Use ci-triage output as the log source if available.
            logInput := priorOutput(prior, "ci-triage")
            runner := logpattern.RunnerFunc(func(ctx context.Context, prompt string, _ []string) (string, error) {
                return router.For(providers.TierFast).Run(ctx, prompt, nil)
            })
            out, err := logpattern.Run(ctx, logpattern.Config{
                Log:    logInput,
                Runner: runner,
            })
            if err != nil {
                return Result{Stage: "log-pattern", Err: fmt.Errorf("log-pattern: %w", err)}
            }
            return Result{Stage: "log-pattern", Output: out,
                Payload: &LogPatternPayload{Patterns: []string{out}}}
        }),

        "diagnose": StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
            runner := diagnose.RunnerFunc(func(ctx context.Context, prompt string, _ []string) (string, error) {
                return router.For(providers.TierBalanced).Run(ctx, prompt, nil)
            })
            out, err := diagnose.Run(ctx, diagnose.Config{
                RepoPath: cfg.RepoPath,
                Log:      priorOutput(prior, "ci-triage"),
                Runner:   runner,
            })
            if err != nil {
                return Result{Stage: "diagnose", Err: fmt.Errorf("diagnose: %w", err)}
            }
            return Result{Stage: "diagnose", Output: out,
                Payload: &DiagnosePayload{Summary: out}}
        }),

        "ticket": StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
            runner := ticket.RunnerFunc(func(ctx context.Context, prompt string, _ []string) (string, error) {
                return router.For(providers.TierBalanced).Run(ctx, prompt, nil)
            })
            out, err := ticket.Run(ctx, ticket.Config{
                Prompt: priorOutput(prior, "diagnose") + "\n" + priorOutput(prior, "council"),
                Path:   cfg.RepoPath,
                Runner: runner,
            })
            if err != nil {
                return Result{Stage: "ticket", Err: fmt.Errorf("ticket: %w", err)}
            }
            return Result{Stage: "ticket", Output: out,
                Payload: &TicketPayload{Body: out}}
        }),

        "pr": StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
            rc := repocontext.GatherRepoContext()
            runner := pr.RunnerFunc(func(ctx context.Context, prompt string, _ []string) (string, error) {
                return router.For(providers.TierBalanced).Run(ctx, prompt, nil)
            })
            out, err := pr.Run(ctx, pr.Config{
                Base:        resolveDiffBase(cfg.RepoPath),
                RepoContext: rc,
                Runner:      runner,
            })
            if err != nil {
                return Result{Stage: "pr", Err: fmt.Errorf("pr: %w", err)}
            }
            return Result{Stage: "pr", Output: out,
                Payload: &PRPayload{Body: out}}
        }),

        "meta": StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
            // meta stage synthesizes all prior outputs into agent tasks.
            allOutput := ""
            for _, r := range prior {
                if !r.IsSkipped() && r.Err == nil {
                    allOutput += "## " + r.Stage + "\n" + r.Output + "\n\n"
                }
            }
            out, err := router.For(providers.TierCoding).Run(ctx, allOutput, nil)
            if err != nil {
                return Result{Stage: "meta", Err: fmt.Errorf("meta: %w", err)}
            }
            return Result{Stage: "meta", Output: out}
        }),
    }
}

// priorOutput returns the Output of the first prior Result with the given stage name,
// or an empty string if not found or skipped.
func priorOutput(prior []Result, stage string) string {
    for _, r := range prior {
        if r.Stage == stage && r.Err == nil {
            return r.Output
        }
    }
    return ""
}

// resolveDiffBase returns the most recent git tag or "main".
func resolveDiffBase(repoPath string) string {
    // Delegate to the cmd-layer helper via a simple shell call.
    // This avoids importing exec in the domain layer directly.
    return "main"
}
```

> **Note:** `resolveDiffBase` in `stages_wiring.go` is a stub that returns `"main"`. The
> cmd layer should pass the resolved base as part of `StageWiringConfig` in a follow-up.
> For now this matches existing behavior in `cmd/devkit/main.go::resolveDiffBase`.

- [ ] **Step 4: Run test, fix any compile errors, verify passes**

```bash
go build ./internal/chain/... 2>&1
go test ./internal/chain/... -v -run "TestBuild|TestWired" 2>&1
```
Expected: compiles clean, both tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/chain/stages_wiring.go internal/chain/stages_wiring_test.go
git commit -m "feat(chain): wire stage runners to existing devkit command packages"
```

---

## Task 7: Add readline dependency and session store

**Files:**
- Modify: `go.mod` / `go.sum`
- Create: `internal/repl/session.go`
- Create: `internal/repl/session_test.go`

- [ ] **Step 1: Add readline dependency**

```bash
go get github.com/chzyer/readline
```

Verify it appears in `go.mod`:
```bash
grep readline go.mod
```
Expected: `github.com/chzyer/readline vX.Y.Z`

- [ ] **Step 2: Write the failing session test**

```go
// internal/repl/session_test.go
package repl_test

import (
    "testing"

    "github.com/89jobrien/devkit/internal/chain"
    "github.com/89jobrien/devkit/internal/repl"
    "github.com/stretchr/testify/assert"
)

func TestSessionAccumulatesResults(t *testing.T) {
    s := repl.NewSession()
    r1 := chain.Result{Stage: "council", Output: "council out"}
    r2 := chain.Result{Stage: "ticket", Output: "ticket out"}
    s.Append(r1)
    s.Append(r2)
    assert.Len(t, s.Results(), 2)
    assert.Equal(t, "council", s.Results()[0].Stage)
}

func TestSessionNoContextSkipsAccumulation(t *testing.T) {
    s := repl.NewSession()
    s.Append(chain.Result{Stage: "council", Output: "out"})
    // With --no-context, the result should NOT be appended.
    s.AppendIfContext(chain.Result{Stage: "ci-triage", Output: "ci out"}, false)
    assert.Len(t, s.Results(), 1)
    // With context (default), it is appended.
    s.AppendIfContext(chain.Result{Stage: "ticket", Output: "ticket out"}, true)
    assert.Len(t, s.Results(), 2)
}

func TestSessionContextSummary(t *testing.T) {
    s := repl.NewSession()
    s.Append(chain.Result{Stage: "council", Output: "council output text"})
    summary := s.ContextSummary()
    assert.Contains(t, summary, "council")
    assert.Contains(t, summary, "council output text")
}

func TestSessionClear(t *testing.T) {
    s := repl.NewSession()
    s.Append(chain.Result{Stage: "council", Output: "out"})
    s.Clear()
    assert.Empty(t, s.Results())
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/repl/... -run TestSession 2>&1 | head -5
```
Expected: `cannot find package` or `undefined: repl.NewSession`

- [ ] **Step 4: Implement `session.go`**

```go
// internal/repl/session.go
package repl

import (
    "fmt"
    "strings"

    "github.com/89jobrien/devkit/internal/chain"
)

// Session holds accumulated results across REPL commands.
// Context accumulation is on by default; --no-context opts out per-command.
type Session struct {
    results []chain.Result
}

// NewSession constructs an empty Session.
func NewSession() *Session { return &Session{} }

// Append always adds the result regardless of --no-context.
func (s *Session) Append(r chain.Result) {
    s.results = append(s.results, r)
}

// AppendIfContext adds the result only when useContext is true (default behavior).
// Pass useContext=false when the command was run with --no-context.
func (s *Session) AppendIfContext(r chain.Result, useContext bool) {
    if useContext {
        s.results = append(s.results, r)
    }
}

// Results returns a copy of all accumulated results.
func (s *Session) Results() []chain.Result {
    out := make([]chain.Result, len(s.results))
    copy(out, s.results)
    return out
}

// ContextSummary returns a markdown string of all session results suitable for
// injecting into prompt context for the next command.
func (s *Session) ContextSummary() string {
    if len(s.results) == 0 {
        return ""
    }
    var sb strings.Builder
    sb.WriteString("## Session context\n\n")
    for _, r := range s.results {
        if r.IsSkipped() {
            continue
        }
        if r.Err != nil {
            fmt.Fprintf(&sb, "### %s (failed: %v)\n\n", r.Stage, r.Err)
            continue
        }
        fmt.Fprintf(&sb, "### %s\n\n%s\n\n", r.Stage, r.Output)
    }
    return sb.String()
}

// Clear removes all accumulated results.
func (s *Session) Clear() { s.results = nil }
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/repl/... -v -run TestSession 2>&1
```
Expected: all 4 tests PASS

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/repl/session.go internal/repl/session_test.go
git commit -m "feat(repl): add Session context store with --no-context support"
```

---

## Task 8: REPL readline loop

**Files:**
- Create: `internal/repl/repl.go`
- Create: `internal/repl/repl_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/repl/repl_test.go
package repl_test

import (
    "strings"
    "testing"

    "github.com/89jobrien/devkit/internal/repl"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestParseCommand_Basic(t *testing.T) {
    cmd, args, noCtx := repl.ParseCommand("council --no-context")
    assert.Equal(t, "council", cmd)
    assert.Empty(t, args)
    assert.True(t, noCtx)
}

func TestParseCommand_ChainWithStages(t *testing.T) {
    cmd, args, noCtx := repl.ParseCommand("chain council ci-triage ticket")
    assert.Equal(t, "chain", cmd)
    assert.Equal(t, []string{"council", "ci-triage", "ticket"}, args)
    assert.False(t, noCtx)
}

func TestParseCommand_Empty(t *testing.T) {
    cmd, _, _ := repl.ParseCommand("  ")
    assert.Equal(t, "", cmd)
}

func TestParseCommand_Exit(t *testing.T) {
    cmd, _, _ := repl.ParseCommand("exit")
    assert.Equal(t, "exit", cmd)
}

func TestDispatchUnknownCommand(t *testing.T) {
    s := repl.NewSession()
    out, err := repl.DispatchCommand("nonexistent", []string{}, false, s, repl.DispatchConfig{})
    require.Error(t, err)
    assert.Contains(t, err.Error(), "unknown command")
    assert.Empty(t, out)
}

func TestDispatchClearResetsSession(t *testing.T) {
    s := repl.NewSession()
    s.Append(makeResult("council"))
    out, err := repl.DispatchCommand("clear", []string{}, false, s, repl.DispatchConfig{})
    require.NoError(t, err)
    assert.Contains(t, out, "cleared")
    assert.Empty(t, s.Results())
}

func TestDispatchContextShowsAccumulated(t *testing.T) {
    s := repl.NewSession()
    s.Append(makeResult("council"))
    out, err := repl.DispatchCommand("context", []string{}, false, s, repl.DispatchConfig{})
    require.NoError(t, err)
    assert.Contains(t, out, "council")
}

func makeResult(stage string) chain.Result {
    return chain.Result{Stage: stage, Output: stage + " output"}
}
```

Note: add `"github.com/89jobrien/devkit/internal/chain"` import to the test file.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/repl/... -run "TestParse|TestDispatch" 2>&1 | head -5
```
Expected: `undefined: repl.ParseCommand`

- [ ] **Step 3: Implement `repl.go`**

```go
// internal/repl/repl.go
package repl

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "strings"
    "syscall"

    "github.com/89jobrien/devkit/internal/chain"
    "github.com/chzyer/readline"
)

// DispatchConfig holds the runtime config for dispatching REPL commands.
// StageRunners and SynthesisRunner are injected by the cmd layer.
type DispatchConfig struct {
    StageRunners    map[string]chain.StageRunner
    SynthesisRunner chain.StageRunner
    RepoPath        string
}

// ParseCommand splits a raw input line into command, args, and --no-context flag.
// Returns empty command string for blank input.
func ParseCommand(line string) (cmd string, args []string, noContext bool) {
    fields := strings.Fields(line)
    if len(fields) == 0 {
        return "", nil, false
    }
    var filtered []string
    for _, f := range fields[1:] {
        if f == "--no-context" {
            noContext = true
        } else {
            filtered = append(filtered, f)
        }
    }
    return fields[0], filtered, noContext
}

// DispatchCommand executes a single REPL command and returns its output.
// Built-in REPL commands: exit, clear, context, help.
// All other commands are delegated to the stage registry or chain pipeline.
func DispatchCommand(cmd string, args []string, noContext bool, s *Session, cfg DispatchConfig) (string, error) {
    switch cmd {
    case "exit", "quit":
        os.Exit(0)
        return "", nil
    case "clear":
        s.Clear()
        return "session cleared.", nil
    case "context":
        summary := s.ContextSummary()
        if summary == "" {
            return "(no session context)", nil
        }
        return summary, nil
    case "help":
        return helpText(), nil
    case "chain":
        return dispatchChain(context.Background(), args, noContext, s, cfg)
    default:
        // Single-stage shorthand: "council" == "chain council"
        if _, ok := cfg.StageRunners[cmd]; ok {
            return dispatchChain(context.Background(), []string{cmd}, noContext, s, cfg)
        }
        return "", fmt.Errorf("unknown command %q — type 'help' for available commands", cmd)
    }
}

func dispatchChain(ctx context.Context, stages []string, noContext bool, s *Session, cfg DispatchConfig) (string, error) {
    slots, err := chain.SelectStages(stages)
    if err != nil {
        return "", err
    }
    for i, slot := range slots {
        if slot.Selected {
            if r, ok := cfg.StageRunners[slot.Name]; ok {
                slots[i].Runner = r
            }
        }
    }
    results, err := chain.RunPipeline(ctx, slots, cfg.SynthesisRunner)
    if err != nil {
        return "", err
    }
    // Accumulate all non-skipped results into session.
    for _, r := range results {
        s.AppendIfContext(r, !noContext)
    }
    // Return the synthesis output as the primary response.
    last := results[len(results)-1]
    if last.Err != nil {
        return "", last.Err
    }
    return last.Output, nil
}

func helpText() string {
    return `devkit repl — available commands:

  chain <stage>...    run selected stages in canonical order + synthesis
  council             shorthand for: chain council
  ci-triage           shorthand for: chain ci-triage
  log-pattern         shorthand for: chain log-pattern
  diagnose            shorthand for: chain diagnose
  ticket              shorthand for: chain ticket
  pr                  shorthand for: chain pr
  meta                shorthand for: chain meta
  context             show accumulated session context
  clear               clear session context
  help                show this message
  exit / quit         exit the REPL

Flags (per command):
  --no-context        run command without reading or writing session context
`
}

// Run starts the readline REPL loop. It blocks until the user exits.
// auth is pre-validated by the cmd layer before Run is called.
func Run(s *Session, cfg DispatchConfig) error {
    rl, err := readline.New("devkit> ")
    if err != nil {
        return fmt.Errorf("repl: readline init: %w", err)
    }
    defer rl.Close()

    // Handle Ctrl-C gracefully.
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sig
        rl.Close()
        os.Exit(0)
    }()

    fmt.Println("devkit repl — type 'help' for commands, 'exit' to quit")
    for {
        line, err := rl.Readline()
        if err != nil { // EOF or Ctrl-D
            break
        }
        line = strings.TrimSpace(line)
        cmd, args, noCtx := ParseCommand(line)
        if cmd == "" {
            continue
        }
        out, err := DispatchCommand(cmd, args, noCtx, s, cfg)
        if err != nil {
            fmt.Fprintf(os.Stderr, "error: %v\n", err)
            continue
        }
        if out != "" {
            fmt.Println(out)
        }
    }
    return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/repl/... -v -run "TestParse|TestDispatch" 2>&1
```
Expected: all 7 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/repl/repl.go internal/repl/repl_test.go
git commit -m "feat(repl): add readline REPL loop with ParseCommand and DispatchCommand"
```

---

## Task 9: `devkit chain` cobra command

**Files:**
- Create: `cmd/devkit/cmd_chain.go`
- Modify: `cmd/devkit/commands_new_test.go` (add chain cmd tests)

- [ ] **Step 1: Write the failing test**

Add to `cmd/devkit/commands_new_test.go`:

```go
func TestChainCmd_Registration(t *testing.T) {
    root := &cobra.Command{Use: "devkit"}
    root.AddCommand(newChainCmd(nil, nil))
    names := map[string]bool{}
    for _, c := range root.Commands() {
        names[c.Name()] = true
    }
    assert.True(t, names["chain"], "chain not registered")
}

func TestChainCmd_HasExpectedFlags(t *testing.T) {
    cmd := newChainCmd(nil, nil)
    assert.NotNil(t, cmd.Flags().Lookup("repo"), "missing --repo flag")
    assert.NotNil(t, cmd.Flags().Lookup("run"), "missing --run flag")
}

func TestChainCmd_UnknownStageErrors(t *testing.T) {
    cmd := newChainCmd(nil, nil)
    _, err := runCmd(t, cmd, "chain", "nonexistent-stage")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "nonexistent-stage")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./cmd/devkit/... -run TestChainCmd 2>&1 | head -5
```
Expected: `undefined: newChainCmd`

- [ ] **Step 3: Implement `cmd_chain.go`**

```go
// cmd/devkit/cmd_chain.go
package main

import (
    "fmt"
    "os"
    "strings"
    "time"

    devlog "github.com/89jobrien/devkit/internal/infra/log"
    "github.com/89jobrien/devkit/internal/chain"
    "github.com/spf13/cobra"
)

// newChainCmd constructs the `devkit chain` command.
// stageRunners and synthesisRunner are injected for testing; pass nil to use production wiring.
func newChainCmd(stageRunners map[string]chain.StageRunner, synthesisRunner chain.StageRunner) *cobra.Command {
    var repo, runID string
    cmd := &cobra.Command{
        Use:   "chain <stage>...",
        Short: "Run selected stages in canonical order, ending with gpt-5.4 synthesis",
        Args:  cobra.MinimumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            // Preflight.
            antKey := os.Getenv("ANTHROPIC_API_KEY")
            oaiKey := os.Getenv("OPENAI_API_KEY")
            gemKey := os.Getenv("GEMINI_API_KEY")

            preErrs := chain.Preflight(chain.PreflightConfig{
                Stages:       args,
                AnthropicKey: antKey,
                OpenAIKey:    oaiKey,
                RepoPath:     repo,
            })
            if len(preErrs) > 0 {
                msgs := make([]string, len(preErrs))
                for i, e := range preErrs {
                    msgs[i] = e.Error()
                }
                return fmt.Errorf("preflight failed:\n  %s", strings.Join(msgs, "\n  "))
            }

            // Build runners if not injected (production path).
            runners := stageRunners
            synth := synthesisRunner
            if runners == nil {
                runners = chain.BuildStageRunners(chain.StageWiringConfig{
                    RepoPath:     repo,
                    RunID:        runID,
                    AnthropicKey: antKey,
                    OpenAIKey:    oaiKey,
                    GeminiKey:    gemKey,
                })
            }
            if synth == nil {
                synth = chain.NewSynthesisRunner(oaiKey, "")
            }

            // Build slot list.
            slots, err := chain.SelectStages(args)
            if err != nil {
                return err
            }
            for i, slot := range slots {
                if slot.Selected {
                    if r, ok := runners[slot.Name]; ok {
                        slots[i].Runner = r
                    }
                }
            }

            sha := devlog.GitShortSHA()
            id := devlog.Start("chain", map[string]string{"stages": strings.Join(args, ",")})
            start := time.Now()

            results, err := chain.RunPipeline(cmd.Context(), slots, synth)
            if err != nil {
                return err
            }

            // Print each non-skipped result.
            for _, r := range results {
                if r.IsSkipped() {
                    continue
                }
                if r.Err != nil {
                    fmt.Fprintf(cmd.OutOrStdout(), "\n## %s (FAILED: %v)\n\n", r.Stage, r.Err)
                    continue
                }
                fmt.Fprintf(cmd.OutOrStdout(), "\n## %s\n\n%s\n", r.Stage, r.Output)
            }

            last := results[len(results)-1]
            devlog.Complete(id, "chain", map[string]string{"stages": strings.Join(args, ",")}, last.Output, time.Since(start))
            _, _ = devlog.SaveCommitLog(sha, "chain", last.Output, map[string]string{"stages": strings.Join(args, ",")})
            return nil
        },
    }
    cmd.Flags().StringVar(&repo, "repo", "", "Repo path (default: cwd)")
    cmd.Flags().StringVar(&runID, "run", "", "GitHub Actions run ID for ci-triage stage")
    return cmd
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./cmd/devkit/... -v -run TestChainCmd 2>&1
```
Expected: all 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/devkit/cmd_chain.go cmd/devkit/commands_new_test.go
git commit -m "feat(cmd): add devkit chain command with preflight and pipeline wiring"
```

---

## Task 10: `devkit repl` cobra command and main.go wiring

**Files:**
- Create: `cmd/devkit/cmd_repl.go`
- Modify: `cmd/devkit/main.go` (register `chain` and `repl`)
- Modify: `cmd/devkit/commands_new_test.go` (add repl cmd tests)

- [ ] **Step 1: Write the failing test**

Add to `cmd/devkit/commands_new_test.go`:

```go
func TestReplCmd_Registration(t *testing.T) {
    root := &cobra.Command{Use: "devkit"}
    root.AddCommand(newReplCmd())
    names := map[string]bool{}
    for _, c := range root.Commands() {
        names[c.Name()] = true
    }
    assert.True(t, names["repl"], "repl not registered")
}

func TestReplCmd_HasExpectedFlags(t *testing.T) {
    cmd := newReplCmd()
    assert.NotNil(t, cmd.Flags().Lookup("repo"), "missing --repo flag")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./cmd/devkit/... -run TestReplCmd 2>&1 | head -5
```
Expected: `undefined: newReplCmd`

- [ ] **Step 3: Implement `cmd_repl.go`**

```go
// cmd/devkit/cmd_repl.go
package main

import (
    "fmt"
    "os"

    "github.com/89jobrien/devkit/internal/chain"
    "github.com/89jobrien/devkit/internal/repl"
    "github.com/spf13/cobra"
)

func newReplCmd() *cobra.Command {
    var repo string
    cmd := &cobra.Command{
        Use:   "repl",
        Short: "Start an interactive REPL session with persistent auth and context",
        RunE: func(cmd *cobra.Command, args []string) error {
            antKey := os.Getenv("ANTHROPIC_API_KEY")
            oaiKey := os.Getenv("OPENAI_API_KEY")
            gemKey := os.Getenv("GEMINI_API_KEY")

            if antKey == "" && oaiKey == "" {
                return fmt.Errorf("repl: ANTHROPIC_API_KEY or OPENAI_API_KEY required")
            }
            if oaiKey == "" {
                return fmt.Errorf("repl: OPENAI_API_KEY required for synthesis stage")
            }

            runners := chain.BuildStageRunners(chain.StageWiringConfig{
                RepoPath:     repo,
                AnthropicKey: antKey,
                OpenAIKey:    oaiKey,
                GeminiKey:    gemKey,
            })
            synth := chain.NewSynthesisRunner(oaiKey, "")

            session := repl.NewSession()
            cfg := repl.DispatchConfig{
                StageRunners:    runners,
                SynthesisRunner: synth,
                RepoPath:        repo,
            }
            return repl.Run(session, cfg)
        },
    }
    cmd.Flags().StringVar(&repo, "repo", "", "Repo path (default: cwd)")
    return cmd
}
```

- [ ] **Step 4: Register both commands in `main.go`**

Find the line in `cmd/devkit/main.go` where commands are added to root (look for `root.AddCommand`):

```bash
grep -n "AddCommand" cmd/devkit/main.go | head -20
```

Add after the existing `AddCommand` calls:

```go
root.AddCommand(newChainCmd(nil, nil))
root.AddCommand(newReplCmd())
```

- [ ] **Step 5: Build and run tests**

```bash
go build ./cmd/devkit/... 2>&1
go test ./cmd/devkit/... -v -run "TestReplCmd|TestChainCmd" 2>&1
```
Expected: compiles clean, all tests PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/devkit/cmd_repl.go cmd/devkit/main.go cmd/devkit/commands_new_test.go
git commit -m "feat(cmd): add devkit repl command and register chain+repl in main"
```

---

## Task 11: Full test suite and install

- [ ] **Step 1: Run full test suite**

```bash
go test ./... 2>&1 | tail -20
```
Expected: all packages pass, no new failures

- [ ] **Step 2: Install binary**

```bash
GOBIN=$HOME/go/bin go install ./cmd/devkit ./cmd/ci-agent ./cmd/meta
```

- [ ] **Step 3: Smoke test chain**

```bash
op run --account=my.1password.com --env-file=$HOME/.secrets -- devkit chain council ticket
```
Expected: preflight passes, council runs, ticket runs, synthesis output printed

- [ ] **Step 4: Smoke test repl**

```bash
op run --account=my.1password.com --env-file=$HOME/.secrets -- devkit repl
```
Expected: `devkit>` prompt appears; type `help` to see commands; type `exit` to quit

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "feat: devkit repl + chain pipeline — persistent session, fixed stage order, gpt-5.4 synthesis"
```

---

## Self-Review

### Spec Coverage

| Requirement | Task |
|------------|------|
| Persist auth (no re-auth per command) | Task 8, 10 — keys resolved once in `newReplCmd`, passed to all runners |
| Accumulate session context | Task 7 — `Session`, `Append`, `AppendIfContext` |
| `--no-context` opt-out | Task 7, 8 |
| Fixed canonical pipeline order | Task 2 — `canonicalOrder`, `SelectStages` |
| Skipped stages leave nil `Result` slot | Task 2, 5 — `IsSkipped()`, zero `Result` |
| Error text in nil slot | Task 5 — `Result.Err` populated, `IsSkipped()=false` for errored stages |
| Synthesis always last, always runs | Task 4, 5 |
| Synthesis uses gpt-5.4 | Task 4 — `synthesisModel = "gpt-5.4"` |
| Preflight always first, validates all failures at once | Task 3 |
| Preflight validates: env, stage names, binaries, repo path | Task 3 |
| Universal `Result` envelope with typed `Payload` | Task 1 |
| `chain <stages...>` command | Task 9 |
| `repl` command | Task 10 |
| Single-stage shorthand in REPL (`council` == `chain council`) | Task 8 |
| `clear`, `context`, `help`, `exit` REPL builtins | Task 8 |

### Type Consistency Check

- `chain.Result` — defined Task 1, used Tasks 2–11: ✓
- `chain.StageRunner` / `StageRunnerFunc` — defined Task 2, used Tasks 5–10: ✓
- `chain.StageSlot` — defined Task 2, used Tasks 5, 9: ✓
- `chain.SelectStages` — defined Task 2, used Tasks 9, 8: ✓
- `chain.RunPipeline(ctx, []StageSlot, StageRunner) ([]Result, error)` — defined Task 5, used Tasks 8, 9: ✓
- `chain.BuildStageRunners(StageWiringConfig) map[string]StageRunner` — defined Task 6, used Tasks 9, 10: ✓
- `chain.NewSynthesisRunner(apiKey, baseURL string) *SynthesisRunner` — defined Task 4, used Tasks 9, 10: ✓
- `repl.Session` — defined Task 7, used Task 8, 10: ✓
- `repl.DispatchConfig.StageRunners map[string]chain.StageRunner` — defined Task 8, used Task 10: ✓
- `newChainCmd(stageRunners map[string]chain.StageRunner, synthesisRunner chain.StageRunner)` — defined Task 9, used Task 10: ✓

No placeholder text found. All code blocks are complete.
