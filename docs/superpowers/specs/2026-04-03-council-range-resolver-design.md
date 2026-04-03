# Council Range Resolver — Design Spec

**Date:** 2026-04-03
**Items:** devkit-14 (centralize range resolution), devkit-15 (HEAD~1 safety + tests)
**Status:** Approved

## Problem

`gitDiff`, `gitLog`, and `gitStat` in `cmd/devkit/main.go` each call `onMainAtBase`
independently, producing 9+ subprocess calls per council run. `onMainAtBase` collapses
all errors to `false` (silent wrong behavior on permission or ref errors). `HEAD~1` is
used unconditionally on single-commit and shallow-clone repos, silently producing empty
output. None of the git helper errors are surfaced to callers.

## Design

### Hexagonal boundaries

A new package `internal/infra/git` owns all git subprocess interaction. It exposes:

1. A **port** (`RangeResolver` interface) that callers depend on.
2. A **domain type** (`RangeResult`) that carries the resolved range and fallback flag.
3. A **production adapter** (`ExecRangeResolver`) that shells out to git.
4. **Query helpers** (`Diff`, `Log`, `Stat`) that accept a `RangeResult`.

`cmd/devkit/main.go` (composition root) wires `ExecRangeResolver` and passes it to
commands. No command imports a concrete git type — only the interface and helpers.

### Types

```go
// internal/infra/git/git.go

// RangeResult is the resolved git revision range for diff/log/stat operations.
type RangeResult struct {
    Range    string // e.g. "HEAD~1...HEAD" or "main...HEAD"
    Fallback bool   // true when base==HEAD and HEAD~1 path was taken
}

// RangeResolver is the port for resolving a git revision range.
type RangeResolver interface {
    ResolveRange(base string) (RangeResult, error)
}
```

### Adapter

```go
// ExecRangeResolver implements RangeResolver using git subprocesses.
type ExecRangeResolver struct{}

func (ExecRangeResolver) ResolveRange(base string) (RangeResult, error)
```

Resolution logic:

1. Resolve `base` and `HEAD` to SHAs via `git rev-parse`.
2. If they differ → return `{Range: base+"...HEAD", Fallback: false}`.
3. If they are equal (base == HEAD) → verify `HEAD~1` exists via
   `git rev-parse --verify HEAD~1`. If it does → return
   `{Range: "HEAD~1...HEAD", Fallback: true}`. If it does not (single-commit
   repo) → return an explicit error rather than empty output.
4. Any subprocess failure → return error (no silent collapse).

### Query helpers

```go
func Diff(r RangeResult) (string, error)
func Log(r RangeResult) (string, error)
func Stat(r RangeResult) (string, error)
```

Each runs the appropriate `git` command with `r.Range` and returns `(output, error)`.
Callers decide how to handle errors — no silent swallowing.

### Call sites

`councilCmd` and `reviewCmd` in `cmd/devkit/main.go` each call `ResolveRange(base)`
once at the top of `RunE`, check for error, then pass `RangeResult` to
`git.Diff`/`git.Log`/`git.Stat`. The old `onMainAtBase`, `gitDiff`, `gitLog`,
`gitStat` functions are deleted.

### Error handling policy

| Scenario | Current behaviour | New behaviour |
|---|---|---|
| Subprocess failure in `onMainAtBase` | Silently returns `false` | Error propagated to caller |
| Single-commit repo (no `HEAD~1`) | Empty output, no error | Error returned: "no parent commit" |
| `gitDiff`/`gitLog`/`gitStat` failures | Silently returns `""` | `(string, error)` — caller handles |
| Base ref not found | `validateRef` error (existing) | Unchanged |

## Testing

Tests live in `internal/infra/git/git_test.go`. Each scenario creates a real git repo
in `t.TempDir()` using `git init` + minimal commits — no mocks for the adapter itself.

| Test | Setup | Expected |
|---|---|---|
| `TestResolveRange_NormalBranch` | feature branch with 1 commit ahead of main | `{Range: "main...HEAD", Fallback: false}` |
| `TestResolveRange_MainAtBase` | on main, base=="main", ≥2 commits | `{Range: "HEAD~1...HEAD", Fallback: true}` |
| `TestResolveRange_SingleCommit` | repo with exactly 1 commit | error (no parent) |
| `TestResolveRange_DetachedHEAD` | `git checkout --detach HEAD` | `{Range: "HEAD~1...HEAD", Fallback: true}` or normal branch result depending on base |

A stub `RangeResolver` in `cmd/devkit/main_test.go` covers command-level wiring without
subprocesses.

## Files changed

| File | Change |
|---|---|
| `internal/infra/git/git.go` | New — port, types, adapter, query helpers |
| `internal/infra/git/git_test.go` | New — four scenario tests |
| `cmd/devkit/main.go` | Delete `onMainAtBase`, `gitDiff`, `gitLog`, `gitStat`; wire `ExecRangeResolver`; update `councilCmd`/`reviewCmd` |
| `cmd/devkit/main_test.go` | Add stub-based wiring test |

## Out of scope

- Changing `resolveDiffBase` / `resolveChangelogBase` (different concern — tag detection)
- Changing `validateRef` (already correct)
- Any council prompt or output changes
