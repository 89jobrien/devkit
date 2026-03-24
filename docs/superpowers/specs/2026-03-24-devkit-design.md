# devkit — Design Spec

**Date:** 2026-03-24
**Status:** Approved
**Source:** Extracted from minibox `scripts/` and `.gitea/workflows/`

---

## Overview

`devkit` is a standalone toolkit (`~/dev/devkit/`) that provides a self-correcting CI/agent workflow extractable to any project. It ships a set of Python agent scripts (using the Claude Agent SDK) plus CI workflow templates. Projects install it via `install.sh`, which copies a self-contained scaffold and writes a `.devkit.json` receipt for future upgrades.

---

## Goals

- Portable: any git project can adopt the full workflow in under two minutes
- Self-contained after install: no runtime dependency on `~/dev/devkit/`
- Self-correcting: CI failures automatically produce LLM-diagnosed Gitea/GitHub issues
- Upgradeable: `upgrade.sh` can re-apply devkit updates while preserving per-project config

---

## Repository Structure

```
devkit/
├── install.sh                  # scaffold installer
├── upgrade.sh                  # re-apply devkit updates to installed project
├── Justfile                    # devkit-internal recipes (dogfooding)
│
├── lib/
│   └── agent_log.py            # shared telemetry; imported by all agents via sys.path
│
├── agents/
│   ├── council.py              # multi-role branch analysis (3–5 roles + synthesis)
│   ├── meta_agent.py           # designer → parallel agents → synthesis
│   ├── ai_review.py            # diff review (security / correctness / breaking changes)
│   └── ci_agent/
│       ├── __init__.py
│       ├── __main__.py         # CI failure diagnosis orchestrator
│       ├── providers.py        # LLM fallback chain: Anthropic → OpenAI → Gemini
│       └── issues.py           # Gitea + GitHub issue/status APIs (platform-branched)
│
└── ci/
    ├── gitea.yml               # Gitea Actions workflow template
    └── github.yml              # GitHub Actions workflow template
```

---

## install.sh

**Invocation:** `~/dev/devkit/install.sh` (run from target project root)

**Prompts:**
1. Project name — default: current directory basename → sets log namespace `~/.dev-agents/<name>/`
2. One-line project description → burned into `ci_agent/__main__.py` as `PROJECT_DESCRIPTION`
3. CI platform: `gitea` | `github` | `both`
4. Components: all selected by default; user can deselect any

**Language detection** (from project root files):
- `Cargo.toml` → test command: `cargo test --workspace`
- `pyproject.toml` / `setup.py` → `uv run pytest`
- `package.json` → `bun test`
- `go.mod` → `go test ./...`
- fallback → leaves `YOUR_TEST_COMMAND` as literal placeholder

**Output:**
```
scripts/
├── .devkit.json          # receipt: version, project_name, description, components, date
├── lib/agent_log.py
├── council.py
├── meta_agent.py
├── ai_review.py
└── ci_agent/
    ├── __init__.py
    ├── __main__.py
    ├── providers.py
    └── issues.py

.gitea/workflows/ci.yml   # created if gitea or both; skipped with warning if exists
.github/workflows/ci.yml  # created if github or both; skipped with warning if exists
```

Prints a Justfile snippet to stdout (not written directly to avoid clobbering existing Justfile).

**CI file safety:** never overwrites an existing CI file — prints a warning and the file path to merge manually.

---

## upgrade.sh

**Invocation:** `~/dev/devkit/upgrade.sh` (run from target project root)

Reads `scripts/.devkit.json` to determine which components are installed and the project's name/description. Re-copies:
- `scripts/lib/agent_log.py`
- `scripts/ci_agent/providers.py` (no project config)
- `scripts/ci_agent/issues.py` (no project config)
- CI templates (only if missing — never overwrites)

Leaves untouched (identified by `# --- PROJECT CONFIG ---` blocks):
- `PROJECT_DESCRIPTION` in `ci_agent/__main__.py`
- `REVIEW_FOCUS` in `ai_review.py`

Updates `version` in `.devkit.json` to the current devkit version.

---

## Agent Scripts

All scripts use `#!/usr/bin/env -S uv run` + PEP 723 inline dependency declarations. No pip install step required. All import `agent_log` via `sys.path.insert(0, scripts/lib)`.

### `agent_log.py`

Two sinks:
- `~/.dev-agents/<project>/agent-runs.jsonl` — structured JSONL telemetry (run start + completion, duration, output)
- `~/.dev-agents/<project>/ai-logs/<sha>-<script>.md` — per-commit markdown archives

Project name resolved from `DEVKIT_PROJECT` env var → fallback to `git rev-parse --show-toplevel` basename.

### `council.py`

Multi-role branch analysis. Roles:
- **Strict Critic** — conservative health score, risks and code smells
- **Creative Explorer** — innovation opportunities, architectural potential
- **General Analyst** — balanced, evidence-based, gaps vs. conventions
- **Security Reviewer** (extensive mode) — generic attack surface: injection, path traversal, auth bypasses, secrets in code, dependency vulnerabilities
- **Performance Analyst** (extensive mode) — allocations, blocking in async, algorithmic complexity

Synthesis agent: health scores (Strict Critic 1.5× weight), consensus, tensions, ranked recommendations.

Modes: `--mode core` (3 roles), `--mode extensive` (5 roles). `--base` defaults to `main`.

### `meta_agent.py`

Fully generic. Workflow:
1. Collect repo context (CLAUDE.md / AGENTS.md / README.md + git log + structure)
2. Fetch + cache Claude Agent SDK docs (24h TTL, `~/.dev-agents/cache/sdk-docs.md`)
3. Designer agent → JSON plan (2–5 independent agents)
4. Execute all agents concurrently via `asyncio.gather`
5. Synthesis agent → final report

No project-specific customization needed.

### `ai_review.py`

Has a `# --- PROJECT CONFIG ---` block with `REVIEW_FOCUS` (multiline string).

Default focus (universally applicable):
- Security: injection, path traversal, auth bypasses, secrets leakage
- Correctness: error handling, silent failures, resource cleanup
- Breaking changes: public API / protocol / schema changes
- Unsafe patterns: language-specific (e.g., `unsafe` in Rust, `eval` in Python)

`install.sh` appends language-specific focus lines based on detected project type.

### `ci_agent/__main__.py`

Two `# --- PROJECT CONFIG ---` blocks:
1. `PROJECT_DESCRIPTION` — one-liner burned in by `install.sh`
2. `PROJECT_CONTEXT_FILES` — list of files to auto-read at runtime (default: `["CLAUDE.md", "AGENTS.md", "README.md"]`)

Runtime context construction: reads each file in `PROJECT_CONTEXT_FILES` (silently skips missing), prepends to diagnosis prompt. Combined with `PROJECT_DESCRIPTION`, this gives the LLM project-specific grounding without requiring per-run configuration.

### `ci_agent/providers.py`

Unchanged from minibox. Fallback chain: Anthropic → OpenAI → Gemini using stdlib `urllib` (no httpx/requests). Raises `DiagnosisUnavailable` if all fail; caller falls back to raw logs.

### `ci_agent/issues.py`

Platform-branched on `GITHUB_ACTIONS` env var:
- **Gitea**: uses `GITEA_URL` + `CI_AGENT_TOKEN` + Gitea REST API
- **GitHub**: uses `GITHUB_TOKEN` + GitHub REST API (`/repos/{owner}/{repo}/...`)

Same interface either way: `set_commit_status`, `ensure_label_exists`, `find_issue_for_commit`, `create_issue`, `add_comment`.

Deduplication: issues contain `<!-- sha: {sha} -->` marker; re-runs on the same commit append a comment rather than opening a new issue.

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
        run: YOUR_TEST_COMMAND   # replaced by install.sh if language detected

  diagnose:
    name: Diagnose Failures
    runs-on: ubuntu-latest
    needs: [test]
    if: failure()
    steps:
      - uses: actions/checkout@v4
      - name: run diagnosis agent
        env:
          GITEA_URL: http://your-gitea-host
          CI_AGENT_TOKEN: ${{ secrets.CI_AGENT_TOKEN }}
          REPO: ${{ gitea.repository }}
          RUN_ID: ${{ gitea.run_id }}
          COMMIT_SHA: ${{ gitea.sha }}
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
          GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}
        run: python3 -m scripts.ci_agent
```

### GitHub (`.github/workflows/devkit-ci.yml`)

Same structure with `github.repository`, `github.run_id`, `github.sha` context variables and `GITHUB_TOKEN` for the issues API.

---

## Logging & Telemetry

All agent runs emit two records to `~/.dev-agents/<project>/agent-runs.jsonl`:
1. On start: `{run_id, script, args, status: "running"}`
2. On complete: `{run_id, script, args, status: "complete", duration_s, output}`

Per-commit markdown logs written to `~/.dev-agents/<project>/ai-logs/<sha>-<script>.md`.

---

## .devkit.json Receipt

```json
{
  "version": "0.1.0",
  "project_name": "myproject",
  "description": "A one-line description of the project",
  "ci_platform": "both",
  "components": ["council", "meta-agent", "ai-review", "ci-agent"],
  "installed_at": "2026-03-24T00:00:00"
}
```

---

## Justfile Snippet (printed by install.sh)

```just
# devkit agent commands
council base="main" mode="core":
    uv run scripts/council.py --base {{base}} --mode {{mode}}

ai-review base="main":
    uv run scripts/ai_review.py --base {{base}}

meta-agent task:
    uv run scripts/meta_agent.py "{{task}}"
```

---

## Out of Scope

- `diagnose.py`: minibox-specific container failure diagnosis; not included (too domain-specific; users add their own)
- `dashboard.py`: minibox-specific TUI reading bench results; not included
- `bench-agent.py`: minibox-specific benchmark analysis; not included
- `commit-msg.py`: commit message generation; not included (may be a future component)
- Windows CI runner support
