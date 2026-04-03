# devkit — Design Spec

**Date:** 2026-03-23
**Status:** Approved
**Module:** `github.com/89jobrien/devkit`

---

## Overview

`devkit` is a Go CLI toolkit that extracts minibox's self-correcting CI/agent workflow into a reusable scaffold. It ships as a compiled binary (`devkit`) providing council analysis, diff review, and a parallel meta-agent, plus a standalone CI failure diagnosis agent invocable via `go run` in any CI pipeline. A single `install.sh` generates a `.devkit.toml` config and CI workflow files in any target project.

**Go SDK:** `github.com/anthropics/anthropic-sdk-go` (requires Go 1.23+).

No agent-loop framework exists for Go; the SDK provides raw API access. `devkit` implements its own tool-use execution loop with `Read`, `Glob`, and `Grep` tools backed by Go filesystem functions.

---

## Repository Structure

```
devkit/
├── go.mod                         # module github.com/89jobrien/devkit
├── go.sum
├── VERSION                        # semver string, e.g. "1.0.0"
├── install.sh
├── upgrade.sh
├── Justfile
│
├── cmd/
│   ├── devkit/
│   │   └── main.go                # CLI binary: install, council, review, meta, upgrade
│   └── ci-agent/
│       └── main.go                # Standalone CI diagnosis agent (go run in CI)
│
├── internal/
│   ├── log/
│   │   └── log.go                 # Structured JSONL + per-commit markdown logging
│   ├── tools/
│   │   └── tools.go               # Read, Glob, Grep as Anthropic tool definitions + handlers
│   ├── loop/
│   │   └── loop.go                # Tool-use execution loop
│   ├── council/
│   │   └── council.go             # Multi-role branch analysis
│   ├── review/
│   │   └── review.go              # Diff review
│   ├── meta/
│   │   └── meta.go                # Design → parallel agents → synthesize
│   └── platform/
│       ├── platform.go            # Platform interface
│       ├── gitea.go               # Gitea Actions API client
│       └── github.go              # GitHub Actions API client
│
└── ci/
    ├── gitea.yml                  # Gitea Actions workflow template
    └── github.yml                 # GitHub Actions workflow template
```

---

## Configuration: `.devkit.toml`

Lives in the project root. Written by `install.sh`, edited by the user, never overwritten by `upgrade.sh`.

```toml
[project]
name        = "myproject"
description = "One-line description sent to the CI diagnosis agent."
version     = "1.0.0"       # devkit version used at install time
install_date = "2026-03-23"
ci_platforms = ["gitea", "github"]

[context]
files = ["CLAUDE.md", "AGENTS.md", "README.md"]

[review]
focus = """
- Security: injection, auth bypasses, dependency risks
- Correctness: error handling, breaking API changes
- Unsafe patterns: language-specific concerns appended by install.sh
"""

[components]
council   = true
review    = true
meta      = true
ci_agent  = true
```

`review.focus` is the only field whose default is language-specific (appended by install.sh). All other fields are project-invariant.

---

## install.sh

### Run-twice behavior

If `.devkit.toml` already exists, aborts:

```
Error: devkit already installed (.devkit.toml found).
Run ~/dev/devkit/upgrade.sh to update CI templates.
```

### Prompts

1. **Project name** — default: current directory name
2. **One-line description** — written to `project.description`
3. **CI platform** — `gitea`, `github`, or `both`
4. **Components** — multi-select, default all: `council`, `review`, `meta`, `ci-agent`

### What it generates

```
.devkit.toml                       # project config receipt
.gitea/workflows/ci.yml            # if gitea or both (skipped with warning if exists)
.github/workflows/ci.yml           # if github or both (skipped with warning if exists)
```

Prints Justfile snippet to stdout. No automated Justfile modification.

### Language detection

Checks for manifest files in the project root only (top-level; monorepos fall back to `YOUR_TEST_COMMAND`).

| Detected file                 | Test command             | `review.focus` additions               |
| ----------------------------- | ------------------------ | -------------------------------------- |
| `Cargo.toml`                  | `cargo test --workspace` | path traversal, unsafe block soundness |
| `pyproject.toml` / `setup.py` | `uv run pytest`          | injection, deserialization safety      |
| `package.json`                | `bun test`               | prototype pollution, XSS               |
| `go.mod`                      | `go test ./...`          | nil dereference, goroutine leaks       |
| fallback                      | `YOUR_TEST_COMMAND`      | (no additions)                         |

---

## Binary Distribution

```bash
go install github.com/89jobrien/devkit/cmd/devkit@latest
```

The binary reads `.devkit.toml` from the current working directory (or nearest parent containing it). `DEVKIT_PROJECT` env var can override the project name for log namespacing.

---

## Internal: Tool-Use Loop (`internal/loop`)

Since the Go SDK provides raw API access (no agent framework), `devkit` implements its own tool-use loop:

```
RunAgent(ctx, client, prompt, tools) → (string, error)

1. Build messages = [UserMessage(prompt)]
2. Call client.Messages.New(ctx, params{model, maxTokens, messages, tools})
3. Append response to messages
4. If StopReason == "end_turn": extract and return text content
5. For each ToolUseBlock in response: execute tool, collect ToolResultBlock
6. Append NewUserMessage(toolResults...) to messages
7. Goto 2
```

Parallel execution (for council roles and meta-agent workers) uses `golang.org/x/sync/errgroup`.

---

## Internal: Tools (`internal/tools`)

Three tools exposed as `anthropic.ToolUnionParam`:

**`Read`** — reads a file at a given path, returns content as string. Errors on paths outside the working directory tree.

**`Glob`** — matches files against a glob pattern, returns newline-separated list of paths.

**`Grep`** — searches file content for a regex pattern, returns matching lines with file:line context.

All three validate that paths stay within the working directory (no `..` traversal).

---

## `devkit council`

Reads `.devkit.toml` for project name, then runs multi-role branch analysis.

**Core mode** (default, `--mode core`): 3 roles run concurrently via `errgroup`. **Extensive mode** (`--mode extensive`): 5 roles. Synthesis is a sequential step after all roles complete.

Roles:

- **Strict Critic** — conservative, demands evidence, health score 0–1
- **Creative Explorer** — optimistic, surfaces opportunities
- **General Analyst** — balanced, evidence-based
- _(extensive only)_ **Security Reviewer** — attack surface, injection, traversal, auth, dependencies
- _(extensive only)_ **Performance Analyst** — allocations, blocking calls, algorithmic complexity

Synthesis: weighted meta-score (Strict Critic 1.5×, others 1.0×), consensus, tensions, ranked recommendations, verdict.

`--no-synthesis`: role outputs printed in sequence, no synthesis step.

CLI: `devkit council [--base main] [--mode core|extensive] [--no-synthesis]`

Diff: `git diff {base}...HEAD`; falls back to `git diff HEAD`.

---

## `devkit review`

Reads `review.focus` from `.devkit.toml`. Runs a single-agent diff review with `Read`, `Glob`, `Grep` tools.

CLI: `devkit review [--base main]`

Diff: `git diff {base}...HEAD`; falls back to `git diff HEAD`.

---

## `devkit meta`

Caches Claude Agent SDK docs (24h TTL) at `~/.dev-agents/cache/sdk-docs.md`. Source URLs:

- `https://docs.anthropic.com/en/docs/claude-code/sdk`
- `https://docs.anthropic.com/en/docs/claude-code/sdk/sdk-python`

Discovers repo context from any of `CLAUDE.md`, `AGENTS.md`, `README.md` that exist, plus `git log --oneline -20`, `git status --short`, and top-150 file paths.

Flow:

1. **Designer agent** — receives task + repo context + SDK docs, outputs a JSON array of 2–5 agent specs (`name`, `role`, `prompt`, `tools`)
2. **Worker agents** — run concurrently via `errgroup`, each receives its self-contained prompt
3. **Synthesis agent** — combines all outputs into Summary, Key Findings, Recommended Actions, Open Questions

CLI: `devkit meta "task description"`
Flags: `--no-synthesis`, `--refresh-docs`

---

## `devkit upgrade`

1. Runs `go install github.com/89jobrien/devkit/cmd/devkit@latest`
2. Reads current devkit `VERSION`
3. If `.devkit.toml` not found: aborts (not a devkit project)
4. If new VERSION < `.devkit.toml` version: warns and prompts confirmation
5. Regenerates CI workflow files (`ci/gitea.yml`, `ci/github.yml`) from current templates — prompts before overwriting if they exist
6. Updates `project.version` and `project.install_date` in `.devkit.toml`
7. Prints notice of any new components available that are set to `false` in `[components]`

**Known limitation:** Justfile snippet is not auto-updated on upgrade.

---

## Logging (`internal/log`)

Reads `DEVKIT_PROJECT` env var; falls back to `.devkit.toml` project name; falls back to `git rev-parse --show-toplevel` basename.

Writes to:

- `~/.dev-agents/<project>/agent-runs.jsonl` — JSONL telemetry (start + completion)
- `~/.dev-agents/<project>/ai-logs/<sha>-<command>.md` — per-commit markdown archives

```go
func Start(command string, args map[string]string) RunID
func Complete(id RunID, command string, args map[string]string, output string, duration time.Duration)
func SaveCommitLog(sha, command, content string, meta map[string]string) (string, error)
func GitShortSHA() string
```

---

## `cmd/ci-agent` — Standalone CI Diagnosis Agent

Invoked in CI via:

```yaml
run: go run github.com/89jobrien/devkit/cmd/ci-agent@v1.0.0
```

No binary installation required on CI runners. The version tag in the CI YAML pins the agent to the devkit version installed at `install.sh` time; `devkit upgrade` updates this tag when regenerating CI files.

### Flow

1. Read env: `CI_PLATFORM`, `REPO`, `RUN_ID`, `COMMIT_SHA` + platform token
2. Fetch jobs for the run → filter `conclusion == "failure"`
3. Fetch log text per failed job (truncate to last 30,000 bytes if larger)
4. Read `project.description` and `context.files` from `.devkit.toml` in the checkout
5. Read any context files that exist
6. Build prompt: description + context + log sections
7. Call LLM provider fallback chain → `(diagnosis, providerName)`
8. Post commit status `pending` → `failure`/`error`
9. `FindIssueForCommit` → append comment or create issue

### LLM Provider Fallback

Raw HTTP calls (no SDK dependency in the standalone agent, to keep `go run` fast):
Anthropic (`claude-sonnet-4-6`) → OpenAI (`gpt-4.1`) → Gemini (`gemini-2.5-flash`)

Skips provider if API key env var absent. Returns `ErrDiagnosisUnavailable` if all fail.

### Platform API (`platform.go` interface)

Selected via `CI_PLATFORM` env var (`"gitea"` or `"github"`).

Both implement:

- `SetCommitStatus(state, description string)` — context: `ci/agent-diagnosis`
- `EnsureLabelExists()` — creates `ci-failure` label (color `#e11d48`); idempotent
- `FindIssueForCommit(sha string) (int, bool)` — paginates open `ci-failure` issues, searches body for `<!-- sha: {sha} -->`, returns first match
- `CreateIssue(sha, diagnosis, provider string, failedJobs []string, runID string) (int, error)`
- `AddComment(issueNumber int, diagnosis, provider string) error`

**Job log fetching:**

- **Gitea**: `GET {GITEA_URL}/api/v1/repos/{REPO}/actions/runs/{RUN_ID}/jobs` → `GET /jobs/{id}/logs`. Auth: `Authorization: token {CI_AGENT_TOKEN}`.
- **GitHub**: `GET https://api.github.com/repos/{REPO}/actions/runs/{RUN_ID}/jobs` → `GET /jobs/{id}/logs` (follows redirect). Auth: `Authorization: Bearer {GITHUB_TOKEN}`.

---

## CI Workflow Templates

### Gitea (`.gitea/workflows/ci.yml`)

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
          GITEA_URL: http://YOUR_GITEA_HOST:3000 # full base URL, no /api/v1
          CI_AGENT_TOKEN: ${{ secrets.CI_AGENT_TOKEN }}
          REPO: ${{ gitea.repository }}
          RUN_ID: ${{ gitea.run_id }}
          COMMIT_SHA: ${{ gitea.sha }}
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
          GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}
        run: go run github.com/89jobrien/devkit/cmd/ci-agent@v1.0.0
```

### GitHub (`.github/workflows/ci.yml`)

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
        run: go run github.com/89jobrien/devkit/cmd/ci-agent@v1.0.0
```

### Required secrets

| Secret              | Required    | Notes                            |
| ------------------- | ----------- | -------------------------------- |
| `ANTHROPIC_API_KEY` | Recommended | Primary LLM provider             |
| `OPENAI_API_KEY`    | Optional    | Fallback                         |
| `GEMINI_API_KEY`    | Optional    | Fallback                         |
| `CI_AGENT_TOKEN`    | Gitea only  | PAT with repo read + issue write |
| `GITHUB_TOKEN`      | GitHub only | Auto-provided by GitHub Actions  |

---

## Justfile Snippet (printed by install.sh)

```makefile
# devkit recipes — add to your Justfile
council base="main" mode="core":
    devkit council --base {{base}} --mode {{mode}}

review base="main":
    devkit review --base {{base}}

meta task:
    devkit meta "{{task}}"
```

---

## Non-Goals

- No dashboard
- No `diagnose` command (runtime-specific — each project writes its own if needed)
- No GitLab support
- No automated Justfile modification
- No monorepo language detection (top-level manifests only)
