# Spec Review Pipeline — Design Spec

**Date:** 2026-04-10
**Status:** Approved

## Problem

Spec files in `docs/superpowers/specs/` are reviewed manually and inconsistently. There is no
automated multi-agent pass to catch completeness gaps, ambiguous requirements, scope creep, or
quality issues before implementation begins. The council pattern proves this approach works for
code review — the same model applies to spec review.

## Design

### Package: `internal/ai/spec`

Mirrors `internal/ai/council` in structure. Exposes:

- `Runner` interface + `RunnerFunc` adapter (same signature as council)
- `Config` — spec content, source path, per-role runner overrides, synthesis runner
- `Result` — `RoleOutputs map[string]string`, `Synthesis string`
- `Run(ctx, cfg) (*Result, error)` — fans out all six roles concurrently via `errgroup`
- `Synthesize(ctx, outputs, cfg, runner) (string, error)` — seventh synthesis pass
- `ParseHealthScore(text) float64` — same regex extraction as council

No mode concept — all six roles always run.

### Six Roles

All roles use the same structured template: Health Score (0.0–1.0), Summary, role-specific
sections, Recommendations. All findings must cite the spec by section heading
(e.g. `## Architecture`) or line number.

| Key          | Focus                                                                 |
|--------------|-----------------------------------------------------------------------|
| completeness | All required sections present, no TBDs, no empty placeholders        |
| ambiguity    | Requirements interpretable in two or more ways                        |
| scope        | Over-engineering, missing decomposition, scope creep                  |
| critic       | Strict conservative review; caps score at 0.4 if critical gaps found |
| creative     | Optimistic; opportunities the spec opens up                           |
| generalist   | Balanced, evidence-based; gaps and progress indicators                |

### Config

```go
type Config struct {
    Content        string            // full spec file content
    Path           string            // source path (for display)
    Runner         Runner            // default runner for all six roles
    Runners        map[string]Runner // per-role overrides
    SynthesisRunner Runner           // runner for the synthesis pass
}
```

### Model Defaults

Role agents default to `gpt-5.4-mini`. Synthesis defaults to `gpt-5.4`. Both are overridable
via `.devkit.toml` under a `[spec]` section:

```toml
[spec]
role_model      = "gpt-5.4-mini"   # model for the six role agents
synthesis_model = "gpt-5.4"        # model for synthesis
```

The `cmd/devkit` composition root reads these overrides, constructs two OpenAI runners with
the appropriate model IDs, and injects them into `spec.Config`.

### Command: `devkit spec [path]`

- If `path` is provided, reads that file.
- If omitted, auto-discovers the most recently modified `.md` file in `docs/superpowers/specs/`.
- Reads the file content, runs `spec.Run`, streams each role output to stdout with a header,
  then runs `spec.Synthesize` and prints the synthesis.
- Output format mirrors `devkit council`: role label headers, horizontal rules, synthesis block.

### Auto-Discovery

`internal/ai/spec` exports `LatestSpecFile(dir string) (string, error)` — walks `dir`, finds
all `.md` files, returns the path with the most recent modification time. The command uses
`docs/superpowers/specs/` as the default directory, resolved relative to the repo root
(same mechanism council uses to find the git root).

### Hexagonal Boundaries

- `internal/ai/spec` has no imports from `cmd/` or `internal/ai/council`.
- `cmd/devkit` is the composition root: it constructs runners, resolves the spec path, calls
  `spec.Run` and `spec.Synthesize`, and formats output.
- `internal/ai/providers` supplies the OpenAI runners — no new provider code needed.

### Testing

- `Run`: stub `Runner` returning fixed strings; assert all six role keys present in `RoleOutputs`.
- `Synthesize`: stub runner; assert synthesis prompt contains all role outputs.
- `LatestSpecFile`: create temp dir with multiple `.md` files at different mtimes; assert correct
  file returned.
- No real API calls in any test.
