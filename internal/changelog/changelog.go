// Package changelog generates release notes from git commits.
package changelog

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed templates/conventional.md
var conventionalTemplate string

//go:embed templates/prose.md
var proseTemplate string

// Runner is the port for LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds all inputs for a changelog run.
type Config struct {
	Log    string // git log --oneline output
	Format string // "conventional" or "prose"
	Runner Runner
}

// Run generates a changelog from git log using the configured runner.
func Run(ctx context.Context, cfg Config) (string, error) {
	return cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
}

func buildPrompt(cfg Config) string {
	tmpl := conventionalTemplate
	if cfg.Format == "prose" {
		tmpl = proseTemplate
	}

	var sb strings.Builder
	sb.WriteString("You are generating a changelog for a software project.\n\n")
	fmt.Fprintf(&sb, "Git log:\n%s\n\n", ifEmpty(cfg.Log, "(no commits)"))
	sb.WriteString("Output template — fill in every section exactly, do not add or remove sections:\n\n")
	sb.WriteString(tmpl)
	return sb.String()
}

func ifEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
