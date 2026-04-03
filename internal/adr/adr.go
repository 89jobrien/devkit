// internal/adr/adr.go
package adr

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

// Config holds all inputs for an ADR run.
type Config struct {
	Title   string
	Context string // problem statement / context text
	Runner  Runner
}

// Run drafts an Architecture Decision Record using the configured runner.
func Run(ctx context.Context, cfg Config) (string, error) {
	return cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
}

func buildPrompt(cfg Config) string {
	var sb strings.Builder
	sb.WriteString("You are a senior software architect. Draft a concise Architecture Decision Record (ADR) in Markdown.\n\n")
	fmt.Fprintf(&sb, "Title: %s\n\n", cfg.Title)
	if cfg.Context != "" {
		fmt.Fprintf(&sb, "Context/Problem:\n%s\n\n", cfg.Context)
	}
	sb.WriteString("Produce the ADR with exactly these sections (use ## headings):\n")
	sb.WriteString("## Status\n## Context\n## Decision\n## Consequences\n\n")
	sb.WriteString("Be specific and actionable. Keep each section under 150 words.")
	return sb.String()
}
