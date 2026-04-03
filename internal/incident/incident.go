// internal/incident/incident.go
package incident

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

// Config holds all inputs for an incident report run.
type Config struct {
	Description string // incident description
	Logs        string // optional log content
	Runner      Runner
}

// Run produces a structured incident report using the configured runner.
func Run(ctx context.Context, cfg Config) (string, error) {
	return cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
}

func buildPrompt(cfg Config) string {
	var sb strings.Builder
	sb.WriteString("You are an incident response engineer. Produce a structured incident report in Markdown based on the information provided.\n\n")
	fmt.Fprintf(&sb, "### Incident Description\n%s\n\n", cfg.Description)
	if cfg.Logs != "" {
		sb.WriteString("### Supporting Logs\n```\n")
		sb.WriteString(cfg.Logs)
		sb.WriteString("\n```\n\n")
	}
	sb.WriteString("Produce the report with exactly these sections (use ## headings):\n")
	sb.WriteString("## Timeline\n## Root Cause\n## Impact\n## Mitigations Applied\n## Follow-up Actions\n\n")
	sb.WriteString("Be specific. For Follow-up Actions, produce a numbered list with owners and due dates where inferable.")
	return sb.String()
}
