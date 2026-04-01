# devkit

## Architecture
Hexagonal Go: each `internal/` package defines its own `Runner` interface + `RunnerFunc` adapter. `cmd/` wires concrete types. No package imports another's concrete type.

## Anthropic SDK
`anthropic.NewClient()` returns `anthropic.Client` (value type, not pointer). Use `client anthropic.Client`, never `*anthropic.Client`.

## Versioning
Current version in `VERSION` file. CI bumps minor on push to main (`0.N.0`). Patch stays 0. Major requires human sign-off. AI never bumps beyond minor.

## CI templates
`ci/github.yml` and `ci/gitea.yml` — install.sh copies these into the target project. The `diagnose` job uses `ci-agent@<tag>` pinned to the current version.
`.github/workflows/devkit.yml` runs `devkit council` on PRs (posts as comment); requires `ANTHROPIC_API_KEY` + `OPENAI_API_KEY` secrets in repo settings. Council uses provider fallback chain (Anthropic → OpenAI → Gemini) configured via `.devkit.toml` `[providers]`.

## Development
- `go test ./...` — 80 tests across 12 packages, no real API calls (httptest + stub runners)
- `go build ./cmd/devkit ./cmd/ci-agent ./cmd/meta` — verify all three binaries compile
- `devkit diagnose [--service <name>] [--log-cmd <cmd>]` — run LLM diagnosis on local service logs
- Pre-commit hook runs `go build ./cmd/devkit ./cmd/ci-agent && go test ./...`; pre-push hook runs `devkit council --base <merge-base>`; bypass both with `DEVKIT_SKIP_HOOKS=1`
- After code changes always reinstall: `GOBIN=$HOME/go/bin go install ./cmd/devkit ./cmd/meta ./cmd/ci-agent` — stale binaries are a common source of confusing failures

## Pushing
Pre-push hook requires `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` — use op run:
`env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY op run --account=my.1password.com --env-file=$HOME/.secrets -- sh -c 'PATH="$HOME/go/bin:$PATH" git push'`
Never run multiple background pushes concurrently — causes "cannot lock ref" failures.
`go install` writes to mise GOBIN, not `~/go/bin` — use `GOBIN=$HOME/go/bin go install` or prefix PATH.

## tools package
`GlobTool` shells out to `fd`; `GrepTool` shells out to `rg` — both must be installed on the host. `BashTool(maxBytes)` runs `sh -c`; output capped at maxBytes with `[truncated]` suffix, non-zero exit appended as `(exit: ...)` rather than returned as a Go error.

## council package
`council.Config.Runners map[string]Runner` — per-role runner override; falls back to `Runner`. Nil `Runner` + missing override returns error (not panic).
`council.ToolUseInstruction` — exported constant; strip from prompts in tool-less runners.
Council requires a TTY — cannot run as a background task. API keys must be plain env vars, not `op://` references.
Role output templates are embedded in persona strings in `council.go::roles` — each role has a structured markdown skeleton with citation requirement (`filename::func_name:line_no`).

## providers package
`internal/providers` — multi-provider fallback chain: Anthropic → OpenAI → Gemini. `Router.For(tier)` returns `council.Runner`; `Router.AgentRunnerFor(tier, tools)` returns agent-capable runner. Gemini excluded from `TierCoding` (no tool use). Override primary provider in `.devkit.toml` under `[providers] primary = "openai"`. `RouterConfig` accepts `AnthropicURL`/`OpenAIURL`/`GeminiURL` for httptest injection in tests.
OpenAI gpt-5.x requires `max_completion_tokens` not `max_tokens`.

