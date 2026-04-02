package ticket

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed templates/ticket.md
var ticketTemplate string

// Runner is the port for LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds inputs for a ticket run.
type Config struct {
	Prompt string // required
	Path   string // optional: source file/dir, included in prompt for grounding
	Runner Runner
}

// Run generates a structured ticket from the configured input using the runner.
func Run(ctx context.Context, cfg Config) (string, error) {
	if strings.TrimSpace(cfg.Prompt) == "" {
		return "", fmt.Errorf("ticket: Prompt must not be empty")
	}
	return cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
}

func buildPrompt(cfg Config) string {
	var sb strings.Builder
	sb.WriteString("You are generating a structured issue ticket for a software project.\n\n")
	if cfg.Path != "" {
		fmt.Fprintf(&sb, "Source context: %s\n\n", cfg.Path)
		sb.WriteString("You may read files in the repository to ground acceptance criteria in real code.\n\n")
	}
	fmt.Fprintf(&sb, "Request:\n%s\n\n", cfg.Prompt)
	sb.WriteString("Output template — fill in every section exactly, do not add or remove sections:\n\n")
	sb.WriteString(ticketTemplate)
	return sb.String()
}
