# Agent Expansion Design: changelog, lint, explain, test-gen, ticket

**Date:** 2026-04-02
**Status:** Approved

---

## Overview

Add five new subcommands to `devkit`: `changelog`, `lint`, `explain`, `test-gen`, and `ticket`. All follow the established hexagonal pattern: `internal/<name>/` package with a `Runner` interface + `RunnerFunc` adapter, a `Config` struct, a `Run` function, and a `buildPrompt` function. The `cmd/devkit/main.go` wires each into a cobra subcommand.

Two agents use pure prompt calls (no tool use): `changelog` and `lint`. Three use agent runners with read-only tools (ReadTool + GlobTool + GrepTool): `explain`, `test-gen`, and `ticket`.

All agents use `go:embed` to load output templates from `internal/<name>/templates/*.md` files rather than inlining template strings in Go source.

---

## Shared Patterns

### Package structure (per agent)
```
internal/<name>/
  <name>.go          # Runner interface, RunnerFunc, Config, Run, buildPrompt
  <name>_test.go     # stub runner tests
  templates/
    <template>.md    # embedded output template(s)
```

### Embedding pattern
```go
import _ "embed"

//go:embed templates/report.md
var reportTemplate string
```

`buildPrompt` appends the template with the instruction: "Fill in every section of this template exactly. Do not add or remove sections."

### Provider tier
- `changelog`, `lint`: `TierBalanced` — single prompt, no tools
- `explain`, `test-gen`, `ticket`: `TierCoding` via `AgentRunnerFor` with ReadTool + GlobTool + GrepTool

### Logging
All agents call `devlog.Start` / `devlog.Complete` / `devlog.SaveCommitLog` matching the existing pattern in `review`, `standup`, `pr`.

---

## Agent: `changelog`

### Purpose
Generate a changelog from git log. Two output formats: conventional (grouped feat/fix/chore) and human-readable prose (GitHub release notes).

### Config
```go
type Config struct {
    Log    string // git log --oneline output
    Format string // "conventional" or "prose"
    Runner Runner
}
```

### CLI
```
devkit changelog [--base <ref>] [--format conventional|prose]
```
- `--base` defaults to the most recent git tag (`git describe --tags --abbrev=0`), falls back to `main`
- `--format` defaults to `conventional`
- Output to stdout

### Templates
- `templates/conventional.md` — version header, sections: Breaking Changes, Features, Bug Fixes, Chores/Refactors
- `templates/prose.md` — release title, summary paragraph, what changed, breaking changes callout

### buildPrompt
Selects template based on `cfg.Format`. Includes the full git log. Instructs LLM to infer version bump (major/minor/patch) from commit prefixes for the conventional format.

---

## Agent: `lint`

### Purpose
Single-file council review. Faster and cheaper than running full council on a branch. Useful in editor integrations or pre-commit hooks.

### Config
```go
type Config struct {
    File   string // file content
    Path   string // original path, for citation context
    Role   string // council role name, default "strict-critic"
    Runner Runner
}
```

### CLI
```
devkit lint <file> [--role strict-critic|security-reviewer|performance-analyst]
```
- Reads file content in cmd before calling `lint.Run`
- `--role` defaults to `strict-critic`
- Output to stdout

### Templates
- `templates/report.md` — issues table (severity | file:line | description), summary verdict

### buildPrompt
Inlines the council role persona for the selected role (imported from `internal/council` or duplicated as a constant). Appends file content and template. Requires `filename:line_no` citations on every finding.

### Note on role personas
`lint` reuses the role persona strings defined in `internal/council/council.go`. Export them as named constants from the `council` package (e.g. `council.PersonaStrictCritic`) and import them in `lint`. Do not duplicate — drift between lint and council personas would produce inconsistent results.

---

## Agent: `explain`

### Purpose
Explain code in plain English. Two modes: file/symbol comprehension and diff/commit explanation.

### Config
```go
type Config struct {
    // file mode (mutually exclusive with diff mode)
    File   string // file content
    Path   string // file path
    Symbol string // optional: function or type name to focus on

    // diff mode
    Diff string
    Log  string
    Stat string

    Runner Runner
}
```

Mode is inferred: if `File` is non-empty → file mode; if `Diff` is non-empty → diff mode. Exactly one must be set; `Run` returns an error otherwise.

### CLI
```
devkit explain <path> [--symbol <name>]   # file mode
devkit explain --base <ref>               # diff mode
```

### Templates
- `templates/file.md` — What it does, Key types/functions, Dependencies, Usage example
- `templates/diff.md` — What changed, Why (inferred from commits), Impact, Breaking changes

### Agent tools
ReadTool + GlobTool + GrepTool. In file mode the agent follows imports and finds call sites. In diff mode it reads changed files for fuller context.

---

## Agent: `test-gen`

### Purpose
Generate Go test stubs. Two modes: file-targeted (exports of a single file) and diff-targeted (new/changed behavior on a branch).

### Config
```go
type Config struct {
    // file mode
    File string // file content
    Path string // used to locate sibling *_test.go files for style reference

    // diff mode
    Diff string
    Log  string

    Runner Runner
}
```

Mode inferred same as `explain`.

### CLI
```
devkit test-gen <path>          # file mode
devkit test-gen --base <ref>    # diff mode
```
- Output is raw Go test code to stdout — no auto-write

### Templates
- `templates/tests.md` — one `TestXxx` stub per exported symbol, table-driven pattern where appropriate, TODO bodies, package declaration

### Agent tools
ReadTool + GlobTool + GrepTool. Agent finds existing `*_test.go` files in the same package to match style conventions before generating new tests.

---

## Agent: `ticket`

### Purpose
Generate a structured issue ticket. Two modes: code-context (from TODOs/errors in a file) and prompt (from free-text description).

### Config
```go
type Config struct {
    Prompt string // free-text or synthesized from code context
    Path   string // optional: file/dir to search for TODOs, FIXMEs, errors
    Runner Runner
}
```

In code-context mode, cmd reads the file/dir path and sets `Path`; the agent uses GrepTool to find TODOs/FIXMEs/failing assertions. In prompt mode, `Path` is empty and `Prompt` is the user's free-text input.

### CLI
```
devkit ticket "description"        # prompt mode
devkit ticket --from <path>        # code-context mode
devkit ticket                      # reads from stdin
```
- Output to stdout; pipe to `gh issue create --body-file -`

### Templates
- `templates/ticket.md` — Title, Description, Acceptance Criteria (checkboxes), Out of Scope, Suggested Labels

### Agent tools
ReadTool + GlobTool + GrepTool. In code-context mode the agent searches for actionable items. In prompt mode the agent may explore the repo to ground acceptance criteria in real code.

---

## File Layout Summary

```
internal/
  changelog/
    changelog.go
    changelog_test.go
    templates/
      conventional.md
      prose.md
  lint/
    lint.go
    lint_test.go
    templates/
      report.md
  explain/
    explain.go
    explain_test.go
    templates/
      file.md
      diff.md
  testgen/
    testgen.go
    testgen_test.go
    templates/
      tests.md
  ticket/
    ticket.go
    ticket_test.go
    templates/
      ticket.md
```

---

## Out of Scope

- Auto-writing test files to disk (`test-gen` outputs to stdout only)
- Creating GitHub issues or PRs automatically (`ticket` outputs to stdout only)
- BAML streaming variants (separate design)
- Non-Go test generation
