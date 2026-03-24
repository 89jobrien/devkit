// internal/review/review.go
package review

import (
	"context"
	"fmt"
)

// Runner is the port for executing LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds parameters for a diff review.
type Config struct {
	Base   string
	Diff   string
	Focus  string
	Runner Runner
}

// Run executes a single-agent diff review and returns the output.
func Run(ctx context.Context, cfg Config) (string, error) {
	if cfg.Focus == "" {
		cfg.Focus = "- Security: injection, traversal, auth bypasses\n- Correctness: error handling, breaking changes\n- Unsafe patterns"
	}
	prompt := fmt.Sprintf("Review this diff.\n\nFocus areas:\n%s\n\nFor each issue: file + line, severity (critical/major/minor), and a concrete fix.\nIf no issues, say so clearly.\n\n```diff\n%s\n```",
		cfg.Focus, cfg.Diff)

	return cfg.Runner.Run(ctx, prompt, []string{"Read", "Glob", "Grep"})
}
