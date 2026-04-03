// internal/scaffold/scaffold.go
package scaffold

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

// Config holds all inputs for a scaffold run.
type Config struct {
	Name        string // Go package name
	Purpose     string // one-sentence description
	RepoContext string // repo context for pattern matching
	Runner      Runner
}

// Run generates boilerplate for a new Go package following hexagonal architecture.
func Run(ctx context.Context, cfg Config) (string, error) {
	if cfg.Runner == nil {
		return "", fmt.Errorf("scaffold: runner is required")
	}
	return cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
}

func buildPrompt(cfg Config) string {
	var sb strings.Builder
	sb.WriteString("You are an expert Go developer. Generate a complete Go source file for a new package following hexagonal architecture.\n\n")
	fmt.Fprintf(&sb, "Package name: %s\n", cfg.Name)
	fmt.Fprintf(&sb, "Purpose: %s\n\n", cfg.Purpose)
	if cfg.RepoContext != "" {
		fmt.Fprintf(&sb, "Repo context (for pattern matching):\n%s\n\n", cfg.RepoContext)
	}
	sb.WriteString("The generated file must include:\n")
	sb.WriteString("1. `// Package <name> ...` GoDoc comment\n")
	sb.WriteString("2. `Runner` interface with `Run(ctx context.Context, prompt string, tools []string) (string, error)`\n")
	sb.WriteString("3. `RunnerFunc` type that implements `Runner`\n")
	sb.WriteString("4. `Config` struct with fields appropriate for the package purpose, plus a `Runner` field\n")
	sb.WriteString("5. `Run(ctx context.Context, cfg Config) (string, error)` function that calls `cfg.Runner.Run(ctx, buildPrompt(cfg), nil)`\n")
	sb.WriteString("6. `buildPrompt(cfg Config) string` function that constructs the LLM prompt\n\n")
	sb.WriteString("Output ONLY the Go source file content. Use `package " + cfg.Name + "` as the package declaration.")
	return sb.String()
}
