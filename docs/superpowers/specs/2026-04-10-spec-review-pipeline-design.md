# Spec Review Pipeline — Design Spec

**Date:** 2026-04-10
**Status:** Approved

## Problem

Spec files in `docs/superpowers/specs/` are reviewed manually and inconsistently. There is no
automated multi-agent pass to catch completeness gaps, ambiguous requirements, scope creep, or
quality issues before implementation begins. The council pattern proves this approach works for
code review — the same model applies to spec review.

### Non-Goals

- General-purpose document review beyond spec files.
- Prompt engineering framework or reusable review-lens abstraction (future opportunity).
- Trend tracking or historical spec quality scores.

## Design

### Package: `internal/ai/spec`

Mirrors `internal/ai/council` in structure. Exposes:

- `Runner` interface + `RunnerFunc` adapter (same signature as council)
- `Config` — spec content, source path, per-role runner overrides
- `Result` — `RoleOutputs map[string]string`
- `Run(ctx, cfg) (*Result, error)` — fans out all six roles concurrently via `errgroup`
- `Synthesize(ctx, outputs, path, runner) (string, error)` — seventh synthesis pass
- `ParseHealthScore(text) float64` — regex extraction matching council's implementation
- `MetaScore(outputs) float64` — simple average of all role health scores

No mode concept — all six roles always run.

### Six Roles

Canonical role keys, in declaration order:

1. `completeness` — All required sections present, no TBDs, no empty placeholders
2. `ambiguity` — Requirements interpretable in two or more ways
3. `scope` — Over-engineering, missing decomposition, scope creep
4. `critic` — Strict conservative review; caps score at 0.4 if critical gaps found
5. `creative` — Optimistic; opportunities the spec opens up
6. `generalist` — Balanced, evidence-based; gaps and progress indicators

All roles share a structured output template with these required top-level sections:

- `**Health Score:**` — decimal 0.0-1.0
- `**Summary**` — 2-4 sentences
- Role-specific sections (see per-role persona definitions in `spec.go`)
- `**Recommendations**` — numbered list, ordered by priority

Roles must not add or remove template sections. Each substantive finding and recommendation
must cite the spec by section heading (e.g. `## Design`) or line number. Bare assertions
without citations are non-compliant. Citation requirements are enforced by prompt instruction
only — no mechanical validation.

### Runtime Semantics

**Concurrency.** `Run` fans out all six roles concurrently via `errgroup.WithContext`. The
errgroup creates a derived context; if any role runner returns an error, that context is
cancelled, which signals remaining in-flight roles to abort.

**Failure behavior.** `Run` returns `(nil, error)` on any role failure — no partial results.
The returned error wraps the first failing role's key and underlying error
(e.g. `role critic: context canceled`). On success, `RoleOutputs` contains exactly six
entries, one per canonical key. The caller must not assume any entries exist when `err != nil`.

**Synthesis failure.** `Synthesize` is called separately after `Run` succeeds. If synthesis
fails, the CLI still has the six role outputs and can display them. Synthesis failure does not
invalidate the role outputs.

**Cancellation.** Both `Run` and `Synthesize` respect the parent context. If the parent
context is cancelled (e.g. SIGINT), in-flight runners receive cancellation via the derived
context.

### ParseHealthScore

Regex: `(?i)\*\*Health Score:?\*\*[:\s]*([\d.]+)`

Extracts the first match from the role output text. Returns `0.5` as the default when no
match is found (same as council). If multiple `**Health Score:**` lines appear, only the
first is used. The parsed value is not clamped — prompt instructions constrain the range.

### Synthesis Contract

`Synthesize(ctx, outputs, path, runner)` builds a prompt containing:

1. All six role outputs, each prefixed with `### {RoleLabel}` and separated by `---`.
   Iteration order is map order (non-deterministic), which is acceptable because the
   synthesis model treats all role inputs equally.
2. The spec file path for context.
3. A required-sections directive specifying the exact output shape:
   - `**Health Scores**` — each role's score plus the meta-score average
   - `**Areas of Consensus**` — findings where 2+ roles agree
   - `**Areas of Tension**` — `"[Role A] sees [X], AND [Role B] sees [Y], suggesting [resolution]"`
   - `**Balanced Recommendations**` — top 3-5 ranked actions
   - `**Spec Health**` — one of: `Ready` / `Almost` / `Needs Work` — with one-line justification

The synthesis model is expected to reconcile disagreements, surface minority viewpoints where
meaningful, and rank recommendations by severity/impact.

### Config

```go
type Config struct {
    Content string            // full spec file content (required)
    Path    string            // source path for display (required)
    Runner  Runner            // default runner for all six roles (required)
    Runners map[string]Runner // per-role overrides (optional, nil = no overrides)
}
```

**Precedence rules:**

1. If `Runners[key]` exists for a role key, that runner is used for that role.
2. Otherwise, `Runner` is used.
3. If the resolved runner is `nil`, `Run` returns an error for that role.
4. `Runners` keys must be canonical role keys. Unknown keys are silently ignored.

The synthesis runner is passed directly to `Synthesize` by the CLI — it is not part of
`Config`. This keeps the spec package unaware of synthesis model selection.

### Model Defaults

Role agents default to `gpt-5.4-mini`. Synthesis defaults to `gpt-5.4`. Both are overridable
via `.devkit.toml` under a `[spec]` section:

```toml
[spec]
role_model      = "gpt-5.4-mini"   # model for the six role agents
synthesis_model = "gpt-5.4"        # model for synthesis
```

**Precedence:** `.devkit.toml` values override hardcoded defaults. If only one field is
present, the other retains its default. Invalid or empty string values are treated as absent
(fall back to default). No environment variable or CLI flag overrides exist — `.devkit.toml`
is the only override mechanism.

The `cmd/devkit` composition root reads these overrides, constructs two OpenAI runners with
the appropriate model IDs, and passes the role runner via `spec.Config.Runner` and the
synthesis runner as a direct argument to `spec.Synthesize`.

### Command: `devkit spec [path]`

**Path resolution:**
- If `path` is provided, reads that file directly.
- If omitted, auto-discovers via `LatestSpecFile` (see below).
- If the file does not exist or is unreadable, exits with code 1 and a descriptive error.

**Output format (deterministic):**
1. If auto-discovered: `Using latest spec: {path}\n\n`
2. For each role in declaration order (`completeness`, `ambiguity`, `scope`, `critic`,
   `creative`, `generalist`):
   - `\n---- {key} ----\n{output}\n`
3. Unless `--no-synthesis` is set:
   - `\n---- SYNTHESIS ----\n{synthesis}\n`
4. `\nMeta Health Score: {score}%\n`
5. `\nLogged to: {log_path}\n`

Note: role outputs are printed in declaration order, not completion order. All roles complete
before any output is printed (buffered, not streaming).

**Exit codes:**
- `0` — all roles completed successfully (synthesis failure still exits 0 if roles succeeded
  — but currently synthesis error propagates as exit 1; this may be relaxed in a follow-up).
- `1` — any error: file not found, role failure, synthesis failure, auto-discovery failure.

**Partial failure:** If any role fails, the entire run fails and no role outputs are printed.
This matches `errgroup` semantics where the first error cancels the group.

### Auto-Discovery

`internal/ai/spec` exports `LatestSpecFile(dir string) (string, error)`.

**Behavior:**
- Scans `dir` non-recursively (top-level files only, using `os.ReadDir`).
- Considers only files with `.md` extension. Skips directories, hidden files, and
  non-markdown files.
- Returns the path with the most recent modification time (filesystem mtime).
- **Tie-breaking:** if multiple files share the same mtime, the lexicographically first
  filename wins (deterministic).
- **Empty directory:** returns `error` with message `no .md files found in {dir}`.
- **Unreadable entries:** skipped silently (consistent with current implementation).

The command resolves the default directory as `docs/superpowers/specs/` relative to the
current working directory.

### Hexagonal Boundaries

- `internal/ai/spec` has no imports from `cmd/` or `internal/ai/council`.
  This means no direct imports — transitive dependencies via shared packages
  (e.g. `golang.org/x/sync/errgroup`) are acceptable.
- `cmd/devkit` is the composition root: it constructs runners, resolves the spec path, calls
  `spec.Run` and `spec.Synthesize`, and formats output.
- `internal/ai/providers` supplies the OpenAI runners — no new provider code needed.

### Testing

**Core unit tests (in `internal/ai/spec`):**
- `Run`: stub `Runner` returning fixed strings; assert all six canonical keys present in
  `RoleOutputs` and values match stub responses.
- `Run` error: stub one role to return error; assert `Run` returns error, no partial results.
- `Run` nil runner: omit `Runner` and `Runners`; assert error for each role.
- `Synthesize`: stub runner; assert synthesis prompt contains all six role labels and outputs.
- `ParseHealthScore`: table-driven tests for valid scores (`0.72`), missing score (returns
  `0.5`), multiple scores (first wins), malformed values.
- `MetaScore`: assert correct average; assert `0` for empty map.
- `LatestSpecFile`: temp dir with multiple `.md` files at different mtimes; assert correct
  file returned.
- `LatestSpecFile` empty dir: assert descriptive error.
- `LatestSpecFile` tie-breaking: two files with identical mtime; assert lexicographically
  first wins.

**Command-level tests (in `cmd/devkit`):**
- Verify path resolution: explicit path used as-is; omitted path triggers auto-discovery.
- Verify model override wiring: `.devkit.toml` values propagate to runner construction.
- Verify `--no-synthesis` flag suppresses synthesis output.

No real API calls in any test. All tests must be fully hermetic.
