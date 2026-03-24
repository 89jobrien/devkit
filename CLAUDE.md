# devkit

## Architecture
Hexagonal Go: each `internal/` package defines its own `Runner` interface + `RunnerFunc` adapter. `cmd/` wires concrete types. No package imports another's concrete type.

## Anthropic SDK
`anthropic.NewClient()` returns `anthropic.Client` (value type, not pointer). Use `client anthropic.Client`, never `*anthropic.Client`.

## Versioning
Current version in `VERSION` file. CI bumps minor on push to main (`0.N.0`). Patch stays 0. Major requires human sign-off. AI never bumps beyond minor.

## CI templates
`ci/github.yml` and `ci/gitea.yml` — install.sh copies these into the target project. The `diagnose` job uses `ci-agent@<tag>` pinned to the current version.
`.github/workflows/devkit.yml` runs `devkit council` on PRs (posts as comment); requires `ANTHROPIC_API_KEY` + `OPENAI_API_KEY` secrets in repo settings.

## Development
- `go test ./...` — 21 tests across 9 packages, no real API calls (httptest + stub runners)
- `go build ./cmd/devkit ./cmd/ci-agent` — verify both binaries compile
- `devkit diagnose [--service <name>] [--log-cmd <cmd>]` — run LLM diagnosis on local service logs
- Pre-push hook runs `devkit review --base main`; bypass with `DEVKIT_SKIP_HOOKS=1`

## Pushing
Pre-push hook requires `ANTHROPIC_API_KEY` — use op run:
`env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY op run --account=my.1password.com --env-file=$HOME/.secrets -- sh -c 'PATH="$HOME/go/bin:$PATH" git push'`
Never run multiple background pushes concurrently — causes "cannot lock ref" failures.
`go install` writes to mise GOBIN, not `~/go/bin` — use `GOBIN=$HOME/go/bin go install` or prefix PATH.

## council package
`council.Config.Runners map[string]Runner` — per-role runner override; falls back to `Runner`. Nil `Runner` + missing override returns error (not panic).
`council.ToolUseInstruction` — exported constant; strip from prompts in tool-less runners (e.g. openAIRunner).
Council requires a TTY — cannot run as a background task. `OPENAI_API_KEY` must be a plain env var, not an `op://` reference.

