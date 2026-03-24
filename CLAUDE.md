# devkit

## Architecture
Hexagonal Go: each `internal/` package defines its own `Runner` interface + `RunnerFunc` adapter. `cmd/` wires concrete types. No package imports another's concrete type.

## Anthropic SDK
`anthropic.NewClient()` returns `anthropic.Client` (value type, not pointer). Use `client anthropic.Client`, never `*anthropic.Client`.

## Versioning
Current version in `VERSION` file. CI bumps minor on push to main (`0.N.0`). Patch stays 0. Major requires human sign-off. AI never bumps beyond minor.

## CI templates
`ci/github.yml` and `ci/gitea.yml` — install.sh copies these into the target project. The `diagnose` job uses `ci-agent@<tag>` pinned to the current version.

## Development
- `go test ./...` — 21 tests across 9 packages, no real API calls (httptest + stub runners)
- `go build ./cmd/devkit ./cmd/ci-agent` — verify both binaries compile
- Pre-push hook runs `devkit review --base main`; bypass with `DEVKIT_SKIP_HOOKS=1`
