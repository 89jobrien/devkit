# BAML Streaming Integration Design

## Goal

Add BAML as a streaming-capable adapter for council roles, producing structured per-role output with live token streaming to stdout. BAML runs alongside existing raw HTTP providers (which remain for fallback); Claude Agent SDK stays for all tool-use agentic loops.

## Architecture

```
cmd/devkit
    │
    ├─ council roles ──► Router.For(tier) ──► council.Runner (port)
    │                                              │
    │                              ┌───────────────┴──────────────────┐
    │                              │                                  │
    │                     baml.Adapter                    providers.chainRunner
    │                   (streaming, structured)          (raw HTTP, existing)
    │
    └─ agent tasks ──► Router.AgentRunnerFor(tier, tools) ──► anthropic SDK (unchanged)
```

The `council.Runner` port is unchanged. BAML is just another adapter behind it. No council/review/diagnose/meta package sees any BAML type.

## BAML Schema

### Location

```
internal/baml/
    baml_src/
        clients.baml       — LLM client definitions per tier
        council.baml       — function + output type definitions
        generators.baml    — Go codegen config
    baml_client/           — generated Go client (committed)
    adapter.go             — council.Runner implementation
    stream.go              — streaming renderer (writes to io.Writer)
```

### Output Types

```baml
class RoleOutput {
    health_score float    @description("0.0-1.0")
    summary string
    recommendations string[]
}

class StrictCriticOutput extends RoleOutput {
    risks string[]
}

class CreativeExplorerOutput extends RoleOutput {
    innovation_opportunities string[]
    architectural_potential string
}

class SecurityReviewerOutput extends RoleOutput {
    findings FindingSeverity[]
}

class FindingSeverity {
    description string
    severity string  @description("critical|high|medium|low|info")
}

class GeneralAnalystOutput extends RoleOutput {
    gaps string[]
    work_patterns string
}
```

### Functions (one per council role)

```baml
function AnalyzeBranchStrictCritic(prompt: string) -> StrictCriticOutput { ... }
function AnalyzeBranchCreativeExplorer(prompt: string) -> CreativeExplorerOutput { ... }
function AnalyzeBranchSecurityReviewer(prompt: string) -> SecurityReviewerOutput { ... }
function AnalyzeBranchGeneralAnalyst(prompt: string) -> GeneralAnalystOutput { ... }
// default fallback for unmapped roles
function AnalyzeBranchDefault(prompt: string) -> RoleOutput { ... }
```

### Clients

```baml
client AnthropicBalanced { provider anthropic, model claude-sonnet-4-6 }
client OpenAIBalanced    { provider openai,    model gpt-5.4 }
client GeminiFast        { provider google-ai, model gemini-3-flash-preview }
// fallback chains via retry_policy or client groups
```

## internal/baml Package

### adapter.go

```go
// Adapter implements council.Runner using BAML streaming.
type Adapter struct {
    role   string
    out    io.Writer   // streaming destination (os.Stdout in prod)
}

func New(role string, out io.Writer) *Adapter

// Run satisfies council.Runner interface.
// Calls the per-role BAML function, streams partial tokens to out,
// then returns the full structured result rendered as markdown string.
func (a *Adapter) Run(ctx context.Context, prompt string) (string, error)
```

### stream.go

```go
// renderStream prints partial tokens as they arrive and accumulates
// the full response for the caller.
func renderStream(ctx context.Context, stream <-chan baml.PartialRoleOutput, out io.Writer) (string, error)
```

## Router Integration

`Router.For(tier)` gains an optional `UseBAML bool` field. When true, it returns a `*baml.Adapter` instead of the chain runner. Default: false (no behaviour change until explicitly enabled).

`cmd/devkit/runner.go` will check `cfg.Providers.UseBAML` (new `.devkit.toml` key) and pass it through.

## Streaming Rendering

The existing council runner collects each role's output and prints a `---- role ----` header before the body. BAML streaming prints tokens live inside that block. The final return value is the complete rendered markdown (same format as today), so the synthesis step is unchanged.

## Testing

- Unit: `internal/baml/adapter_test.go` — mock BAML client, assert `Run()` returns formatted markdown
- Integration: tagged `//go:build integration` — requires real API keys, runs one role end-to-end
- No BAML tests fire real API calls in `go test ./...` (existing CI constraint)

## .devkit.toml

```toml
[providers]
primary = "openai"
use_baml = false   # set true to route council through BAML streaming
```

## Migration Path

Phase 1 (this spec): BAML adapter available, off by default. Enable per-project via `use_baml = true`.

Phase 2 (future): BAML becomes the default for council; raw HTTP chain retained only for fallback if BAML client fails.

Phase 3 (future): raw HTTP providers removed from council path; retained only for `AgentRunnerFor` fallback (non-Anthropic tool-use).

Claude Agent SDK (`anthropic-sdk-go`) is **never replaced** — it handles all tool-use agentic loops in `AgentRunnerFor`.

## Open Questions

None. Design is approved.
