# devkit — Design Spec

**Date:** 2026-03-23
**Status:** Approved

---

## Overview

`devkit` is a standalone toolkit (`~/dev/devkit/`) that extracts minibox's self-correcting CI/agent workflow into a reusable scaffold. It ships a CI failure diagnosis agent, a multi-role branch review council, a meta-agent (parallel agent designer), and a diff review script — all wired into Gitea and/or GitHub Actions. A single `install.sh` copies a fully self-contained `scripts/` directory into any target project.

---

## Repository Structure

```
devkit/
├── VERSION                     # semver string, e.g. "1.0.0"
├── install.sh
├── upgrade.sh
├── Justfile
│
├── lib/
│   └── agent_log.py
│
├── agents/
│   ├── council.py
│   ├── meta_agent.py
│   ├── ai_review.py
│   └── ci_agent/
│       ├── __init__.py
│       ├── __main__.py
│       ├── providers.py
│       └── issues.py
│
└── ci/
    ├── gitea.yml
    └── github.yml
```

---

## PROJECT CONFIG Blocks

Scripts with project-specific content use sentinel comments:

```python
# --- PROJECT CONFIG START ---
PROJECT_DESCRIPTION = "A Linux container runtime written in Rust."
# --- PROJECT CONFIG END ---
```

Only `ci_agent/__main__.py` and `ai_review.py` contain PROJECT CONFIG blocks. All other files are fully devkit-owned.

### upgrade.sh merge algorithm

For files containing PROJECT CONFIG blocks:
1. Read the currently-installed file → extract content between sentinels (the "old config")
2. Read the new file from devkit source → locate the same sentinel pair
3. Replace the content between sentinels in the new file with the old config verbatim
4. Write the result to the installed path

The sentinels themselves are always taken from the new devkit version. If a new devkit version adds a second PROJECT CONFIG block, the block is installed with its default content and a notice is printed: `"New PROJECT CONFIG block added to <file> — review and customize."`

---

## `.devkit.json` Receipt Schema

```json
{
  "version": "1.0.0",
  "install_date": "2026-03-23",
  "project_name": "myproject",
  "project_description": "One-line description baked into CI agent prompt.",
  "ci_platforms": ["gitea", "github"],
  "components": ["council", "meta-agent", "ai-review", "ci-agent"]
}
```

`upgrade.sh` only upgrades components listed in `"components"`. It prints a notice for any new components available in the devkit version being upgraded to, but does not install them automatically. On apparent downgrade (new `VERSION` < receipt `version`), upgrade.sh prints a warning and prompts for confirmation before proceeding.

---

## install.sh

### Run-twice behavior

If `scripts/.devkit.json` already exists, `install.sh` aborts:

```
Error: devkit already installed (scripts/.devkit.json found).
Run ~/dev/devkit/upgrade.sh to update.
```

### Prompts

1. **Project name** — default: current directory name
2. **One-line description** — burned into `PROJECT_DESCRIPTION`
3. **CI platform** — `gitea`, `github`, or `both`
4. **Components** — multi-select, default all: `council`, `meta-agent`, `ai-review`, `ci-agent`

### What it generates

```
scripts/
├── .devkit.json
├── lib/
│   └── agent_log.py
├── council.py              (if selected)
├── meta_agent.py           (if selected)
├── ai_review.py            (if selected)
└── ci_agent/               (if selected)
    ├── __init__.py
    ├── __main__.py         (PROJECT_DESCRIPTION baked in)
    ├── providers.py
    └── issues.py

.gitea/workflows/ci.yml     (if gitea or both — skipped with warning if exists)
.github/workflows/ci.yml    (if github or both — skipped with warning if exists)
```

Prints Justfile snippet to stdout. No automated Justfile modification.

### Language detection

Checks for manifest files in the project root only (top-level). Monorepos with manifests only in subdirectories fall back to `YOUR_TEST_COMMAND` — this is a known limitation.

Applies to: CI template `YOUR_TEST_COMMAND` substitution, and `REVIEW_FOCUS` additions in `ai_review.py`.

| Detected file | Test command | `ai_review.py` focus additions |
|---|---|---|
| `Cargo.toml` | `cargo test --workspace` | path traversal, unsafe block soundness |
| `pyproject.toml` / `setup.py` | `uv run pytest` | injection, deserialization safety |
| `package.json` | `bun test` | prototype pollution, XSS |
| `go.mod` | `go test ./...` | nil dereference, goroutine leaks |
| fallback | `YOUR_TEST_COMMAND` | (no additions) |

---

## Agent Scripts

All use `#!/usr/bin/env -S uv run` + PEP 723 inline deps. Each imports `agent_log` with:

```python
import sys, os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "lib"))
import agent_log
```

This resolves to `scripts/lib/agent_log.py` regardless of working directory.

### `agent_log.py`

Reads `DEVKIT_PROJECT` env var for log namespace. Falls back to `git rev-parse --show-toplevel` basename.

Writes to:
- `~/.dev-agents/<project>/agent-runs.jsonl` — JSONL telemetry
- `~/.dev-agents/<project>/ai-logs/<sha>-<script>.md` — per-commit markdown archives

API:

```python
def log_start(script: str, args: dict) -> str:
    """Write 'running' entry to JSONL. Returns run_id (ISO timestamp)."""

def log_complete(run_id: str, script: str, args: dict, output: str, duration_s: float) -> None:
    """Write 'complete' entry to JSONL."""

def save_commit_log(sha: str, script: str, content: str, meta: dict) -> Path:
    """Write ~/.dev-agents/<project>/ai-logs/<sha>-<script>.md with YAML-style header + content."""

def git_short_sha() -> str:
    """Return output of git rev-parse --short HEAD."""
```

### `council.py`

**Core mode** (`--mode core`, default): 3 roles. **Extensive mode** (`--mode extensive`): 5 roles. Synthesis is a sequential step after all roles complete; it is not a role.

Roles:
- **Strict Critic** — conservative, demands evidence
- **Creative Explorer** — optimistic, surfaces opportunities
- **General Analyst** — balanced, evidence-based
- *(extensive only)* **Security Reviewer** — attack surface, injection, traversal, auth, dependencies
- *(extensive only)* **Performance Analyst** — allocations, blocking calls, algorithmic complexity

Each role returns a health score 0.0–1.0 and structured findings.

**Synthesis**: weighted meta-score where Strict Critic has weight 1.5×, all other roles have weight 1.0×. Outputs: consensus points, dialectic tensions, ranked recommendations, final verdict (Good / Needs work / Significant issues).

**`--no-synthesis`**: role outputs are printed in sequence with separator lines; no synthesis step runs.

CLI: `council.py [--base main] [--mode core|extensive] [--no-synthesis]`

Diff: `git diff {base}...HEAD`; falls back to `git diff HEAD` if no commits ahead of base.

### `meta_agent.py`

Fetches and caches the Claude Agent SDK docs (24h TTL) at `~/.dev-agents/cache/sdk-docs.md`. Source URLs:
- `https://docs.anthropic.com/en/docs/claude-code/sdk`
- `https://docs.anthropic.com/en/docs/claude-code/sdk/sdk-python`

HTML is stripped to plain text before caching. Cache is keyed to the file's mtime; `--refresh-docs` forces re-fetch.

Discovers repo context from any of `CLAUDE.md`, `AGENTS.md`, `README.md` that exist, plus `git log --oneline -20`, `git status --short`, and top-150 file paths (excluding build artifacts).

Flow:
1. **Designer agent** — receives task + repo context + SDK docs, outputs a JSON array of 2–5 agent specs (`name`, `role`, `prompt`, `tools`)
2. **Worker agents** — run concurrently via `asyncio.gather`, each receives its self-contained prompt
3. **Synthesis agent** — combines all outputs into Summary, Key Findings, Recommended Actions, Open Questions

CLI: `meta_agent.py "task"` or `echo "task" | meta_agent.py`
Flags: `--no-synthesis`, `--refresh-docs`

### `ai_review.py`

Contains a `# --- PROJECT CONFIG START/END ---` block:

```python
# --- PROJECT CONFIG START ---
REVIEW_FOCUS = """
- Security: ...
- Correctness: ...
- Breaking changes: ...
- Error handling: ...
"""
# --- PROJECT CONFIG END ---
```

`install.sh` appends language-specific lines to this block.

Diff: `git diff {base}...HEAD`; falls back to `git diff HEAD`.

CLI: `ai_review.py [--base main]`

### `ci_agent/__main__.py`

Contains two `# --- PROJECT CONFIG START/END ---` blocks:

```python
# --- PROJECT CONFIG START ---
PROJECT_DESCRIPTION = "A Python web service."
PROJECT_CONTEXT_FILES = ["CLAUDE.md", "AGENTS.md", "README.md"]
# --- PROJECT CONFIG END ---
```

**Flow:**

1. Read required env vars: `CI_PLATFORM`, `REPO`, `RUN_ID`, `COMMIT_SHA` + platform token
2. Fetch jobs for the run via platform API; filter to `conclusion == "failure"`
3. Fetch log text for each failed job (truncate to last 30 000 bytes if larger)
4. Read any `PROJECT_CONTEXT_FILES` that exist in the checkout
5. Build prompt: project description + context file contents + log sections
6. Call `providers.ask_with_fallback(prompt)` → `(diagnosis, provider_name)`
7. Post commit status `pending` (`ci/agent-diagnosis` context)
8. Run LLM; update commit status to `failure` (or `error` if all providers failed)
9. Call `issues.find_issue_for_commit`; if found, append comment; if not, create issue

**Job log fetching:**

- **Gitea**: `GET {GITEA_URL}/api/v1/repos/{REPO}/actions/runs/{RUN_ID}/jobs?limit=50` → job list; then `GET {GITEA_URL}/api/v1/repos/{REPO}/actions/jobs/{job_id}/logs` → raw log bytes. Auth: `Authorization: token {CI_AGENT_TOKEN}`.
- **GitHub**: `GET https://api.github.com/repos/{REPO}/actions/runs/{RUN_ID}/jobs` → job list; then `GET https://api.github.com/repos/{REPO}/actions/jobs/{job_id}/logs` → redirects to log download URL. Auth: `Authorization: Bearer {GITHUB_TOKEN}`.

### `ci_agent/providers.py`

Fallback chain: Anthropic (`claude-sonnet-4-6`) → OpenAI (`gpt-4.1`) → Gemini (`gemini-2.5-flash`). stdlib `urllib` only. Skips provider if its API key env var is absent. Raises `DiagnosisUnavailable` if all providers fail.

Returns `(diagnosis_text: str, provider_name: str)`.

### `ci_agent/issues.py`

Platform selected via `CI_PLATFORM` env var (`"gitea"` or `"github"`). Both implement:

- `set_commit_status(state, description)` — posts to `ci/agent-diagnosis` context
- `ensure_label_exists()` — creates `ci-failure` label (color `#e11d48`) if absent; idempotent
- `find_issue_for_commit(sha) -> int | None` — paginates open `ci-failure` issues searching body for `<!-- sha: {sha} -->` marker; returns first matching issue number, or `None` if not found
- `create_issue(sha, diagnosis, provider, failed_jobs, run_id) -> int` — creates issue with title `"CI failure: {sha[:8]} — {jobs}"`, body includes diagnosis + run URL + `<!-- sha: {sha} -->` marker
- `add_comment(issue_number, diagnosis, provider)` — appends re-run diagnosis as comment

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
      - name: run diagnosis agent
        env:
          CI_PLATFORM: gitea
          GITEA_URL: http://YOUR_GITEA_HOST:3000   # full base URL, no /api/v1
          CI_AGENT_TOKEN: ${{ secrets.CI_AGENT_TOKEN }}
          REPO: ${{ gitea.repository }}
          RUN_ID: ${{ gitea.run_id }}
          COMMIT_SHA: ${{ gitea.sha }}
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
          GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}
        run: python3 -m scripts.ci_agent
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
        run: python3 -m scripts.ci_agent
```

### Required secrets

| Secret | Required | Notes |
|---|---|---|
| `ANTHROPIC_API_KEY` | Recommended | Primary LLM provider |
| `OPENAI_API_KEY` | Optional | Fallback |
| `GEMINI_API_KEY` | Optional | Fallback |
| `CI_AGENT_TOKEN` | Gitea only | PAT with repo read + issue write permissions |
| `GITHUB_TOKEN` | GitHub only | Auto-provided by GitHub Actions |

---

## upgrade.sh

1. Abort if `scripts/.devkit.json` not found
2. Read `VERSION` from `~/dev/devkit/VERSION`
3. If new version < receipt version: print warning, prompt for confirmation
4. For each component in receipt: re-copy devkit-owned files; apply PROJECT CONFIG merge algorithm for `__main__.py` and `ai_review.py`; print notice for any new PROJECT CONFIG blocks
5. Print notice of new components available in current devkit version that are absent from receipt
6. Update `version` and `install_date` in `scripts/.devkit.json`

**Known limitation:** Justfile snippet is not auto-updated on upgrade. Users compare and apply changes manually.

---

## Justfile Snippet (printed by install.sh)

```makefile
# devkit recipes — add to your Justfile
council base="main" mode="core":
    uv run scripts/council.py --base {{base}} --mode {{mode}}

ai-review base="main":
    uv run scripts/ai_review.py --base {{base}}

meta-agent task:
    uv run scripts/meta_agent.py "{{task}}"
```

---

## Non-Goals

- No dashboard
- No `diagnose.py` (runtime-specific — each project adds its own if needed)
- No Python package / PyPI distribution
- No GitLab support
- No automated Justfile modification
- No monorepo language detection (top-level manifests only)
