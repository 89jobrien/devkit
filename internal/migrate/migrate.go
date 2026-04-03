// internal/migrate/migrate.go
package migrate

import (
	"context"
	"fmt"
	"strings"
)

// Runner is the port for LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds all inputs for a migrate run.
type Config struct {
	Old    string // old API signature or description
	New    string // new API signature or description
	Code   string // file content to update
	Path   string // original file path
	Runner Runner
}

// Run analyzes a breaking API change and suggests callsite updates as a unified diff.
func Run(ctx context.Context, cfg Config) (string, error) {
	return cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
}

func buildPrompt(cfg Config) string {
	var sb strings.Builder
	sb.WriteString("You are an expert Go developer performing an API migration. Analyze the breaking change below and produce a unified diff showing the callsite updates needed.\n\n")
	fmt.Fprintf(&sb, "### Old API\n```\n%s\n```\n\n", cfg.Old)
	fmt.Fprintf(&sb, "### New API\n```\n%s\n```\n\n", cfg.New)
	fmt.Fprintf(&sb, "### File to update: %s\n```go\n%s\n```\n\n", cfg.Path, cfg.Code)
	sb.WriteString("Output a unified diff (`--- a/file` / `+++ b/file` format) with the minimal changes needed to migrate every callsite. ")
	sb.WriteString("If no changes are needed, say so explicitly. Do not rewrite unrelated code.")
	return sb.String()
}
