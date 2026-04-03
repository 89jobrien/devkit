# Repo Maintenance Tools — Design Spec

**Date:** 2026-04-03
**Scope:** Two efforts: (1) reorganize `internal/` into domain subdirectories, (2) four new subcommands for repo health, routine automation, CI triage, and repo-level review.

---

## Part 1: `internal/` Reorganization

### Goal

Group devkit's 24 internal packages into four domain directories for navigability and to create clear homes for new packages.

### Domain Mapping

| Directory | Packages |
|-----------|----------|
| `internal/ai/` | `baml`, `council`, `providers`, `meta`, `loop` |
| `internal/dev/` | `lint`, `explain`, `testgen`, `docgen`, `scaffold`, `migrate`, `adr`, `ticket`, `pr`, `review` |
| `internal/ops/` | `diagnose`, `incident`, `logpattern`, `profile`, `changelog`, `standup` |
| `internal/infra/` | `platform`, `log`, `tools` |

**Rationale:**
- `ai/` — LLM infrastructure: client wrappers, provider routing, streaming, agentic execution
- `dev/` — Developer workflow tools: code review, documentation, test generation, scaffolding
- `ops/` — Operational tools: incident reports, log analysis, CI diagnosis, changelog, standup
- `infra/` — Shared primitives: logging, platform detection, tool definitions

### Execution Plan

1. `git mv` each package directory to its new location
2. Bulk rewrite all import paths across `.go` files:
   - `github.com/89jobrien/devkit/internal/<pkg>` → `github.com/89jobrien/devkit/internal/<domain>/<pkg>`
   - Affected files: all `cmd/devkit/cmd_*.go`, `cmd/devkit/main.go`, `cmd/meta/main.go`, cross-package imports in `providers` → `tools`, `lint` → `council`
3. Gate: `go build ./...` and `go test ./...` must pass

### Import Path Changes

All 23 packages get new paths. Key cross-package deps that must be updated:
- `internal/ai/providers` imports `internal/infra/tools`
- `internal/dev/lint` imports `internal/ai/council`
- `cmd/devkit/` imports all 23 packages

### No Behavior Change

This is a pure mechanical refactor. Package names (the `package` declaration) stay the same — only import paths change.

---

## Part 2: New Subcommands

### 2a. `devkit health` — Repo Health Monitor

**Package:** `internal/ops/health/`
**Purpose:** Run a structured set of checks against a local repo and emit a scored health report.

#### Checks

| Check | Method | Severity |
|-------|--------|----------|
| `CLAUDE.md` present | file existence | warning |
| CI config present (`.github/workflows/` or `.gitea/workflows/`) | file existence | warning |
| Dependency freshness | `go.mod`/`Cargo.toml` mod time vs 90 days | info |
| TODO/FIXME density | count occurrences in source, flag if >20 | info |
| Test files present | at least one `_test.go` or `_test.rs` | warning |
| Devkit CI template version | compare installed `devkit.yml` to current devkit version | warning |

#### BAML Schema (`internal/baml/baml_src/health.baml`)

```
class HealthCheck {
  name        string
  status      string   // pass | warn | fail
  severity    string   // info | warning | critical
  detail      string
  suggestion  string
}

class HealthReport {
  repo        string
  score       int      // 0–100
  summary     string
  checks      HealthCheck[]
}

function AnalyzeRepoHealth(repo_context: string, check_results: string) -> HealthReport
```

#### CLI

```
devkit health [--repo <path>] [--format json|markdown]
```

Default repo is current working directory. Output is markdown by default.

#### Hexagonal Pattern

- `Runner` interface + `RunnerFunc` adapter
- `Config`: `RepoPath string`, `Runner Runner`
- `Run(ctx, cfg)`: gathers check results locally (no LLM), passes summary string to BAML for scoring and suggestions
- Local checks run first (fast, deterministic); BAML call is for narrative summary and scoring only

---

### 2b. `devkit automate` — Routine Maintenance Orchestrator

**Package:** `internal/ops/automate/`
**Purpose:** Run a configurable set of routine maintenance tasks against a repo in one command.

#### Tasks

| Flag value | Delegates to |
|------------|-------------|
| `changelog` | `internal/ops/changelog` |
| `standup` | `internal/ops/standup` |
| `tickets` | `internal/dev/ticket` (scans for TODO/FIXME comments, creates one ticket per) |

#### CLI

```
devkit automate --tasks changelog,standup,tickets [--repo <path>]
```

#### Design

`automate` is a pure orchestrator — no new BAML, no new LLM calls. It constructs `Config` structs for each delegated package and calls their `Run()` functions in sequence, printing section headers between outputs.

`Config`: `Tasks []string`, `RepoPath string`, `Runner Runner` (passed through to each delegated package).

---

### 2c. `devkit ci-triage` — CI Failure Diagnosis

**Package:** `internal/ops/citriage/`
**Purpose:** Pull a CI failure log automatically and run diagnosis, returning structured root cause and fix suggestion.

#### Input Modes

- `--stdin`: read failure log from stdin
- `--run <url-or-id>`: shell out to `gh run view <id> --log-failed` to fetch log
- Default (no flags): attempt `gh run list --limit 1 --json databaseId` to find the most recent failed run

#### BAML Schema (`internal/baml/baml_src/citriage.baml`)

```
class CITriageReport {
  failing_job         string
  root_cause          string
  suggested_fix       string
  reproduction_steps  string[]
  confidence          string   // high | medium | low
}

function TriageCIFailure(log: string, repo_context: string) -> CITriageReport
```

#### CLI

```
devkit ci-triage [--stdin] [--run <id>] [--repo <path>]
```

#### Hexagonal Pattern

Standard: `Runner`, `RunnerFunc`, `Config{ RepoPath, RunID, Runner }`, `Run(ctx, cfg)`.

Log capped at 64KB before passing to BAML (same pattern as `logpattern`).

---

### 2d. `devkit repo-review` — Repo-Level Council Review

**Package:** `internal/dev/reporeview/`
**Purpose:** Run a council-style review scoped to overall repo health rather than a single PR or file.

#### Context Gathered

- `CLAUDE.md` content
- `README.md` (first 2KB)
- Recent git log (last 20 commits, `--oneline`)
- Directory tree (top 2 levels)
- Any failing CI (optional, from `gh run list`)

#### Design

Reuses `internal/ai/council` runner directly — no new BAML. Constructs a prompt from gathered repo context and asks: "What needs attention in this repo? Identify the top issues by priority."

Runs the same role personas as `devkit council` but with repo-context framing instead of diff framing.

#### CLI

```
devkit repo-review [--repo <path>] [--format markdown|json]
```

#### Config

`Config{ RepoPath string, Runner council.Runner }` — uses `council.Runner` directly, not a new interface.

---

## File Inventory

### New packages
- `internal/ops/health/health.go`, `health_test.go`
- `internal/ops/automate/automate.go`, `automate_test.go`
- `internal/ops/citriage/citriage.go`, `citriage_test.go`
- `internal/dev/reporeview/reporeview.go`, `reporeview_test.go`

### New BAML files
- `internal/baml/baml_src/health.baml`
- `internal/baml/baml_src/citriage.baml`

### New command files
- `cmd/devkit/cmd_health.go`
- `cmd/devkit/cmd_automate.go`
- `cmd/devkit/cmd_citriage.go`
- `cmd/devkit/cmd_reporeview.go`

### Modified
- All `cmd/devkit/cmd_*.go` — import path updates (Part 1)
- `cmd/devkit/main.go` — register 4 new subcommands
- BAML client regenerated after new `.baml` files added

---

## Implementation Order

1. Part 1: reorg `internal/` (mechanical, gates on `go build ./...`)
2. Add BAML schemas (`health.baml`, `citriage.baml`) and regenerate client
3. Implement `health` package + command
4. Implement `ci-triage` package + command
5. Implement `automate` package + command
6. Implement `repo-review` package + command
7. Register all commands in `main.go`
8. `go test ./...` gate

---

## Out of Scope

- Cross-repo sync (pushing devkit CI templates to other repos) — separate effort
- Scheduling / cron triggers — `devkit automate` is invoked manually or from CI
- Dependency auto-update PRs — health check flags, doesn't fix
