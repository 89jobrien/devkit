// internal/lint/lint.go
package lint

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/89jobrien/devkit/internal/council"
)

//go:embed templates/report.md
var reportTemplate string

// Runner is the port for LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds all inputs for a lint run.
type Config struct {
	File   string // file content
	Path   string // original file path, for citation context
	Role   string // council role key; defaults to "strict-critic" if empty or unknown
	Runner Runner
}

// Run performs a single-file lint review using the configured runner.
func Run(ctx context.Context, cfg Config) (string, error) {
	return cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
}

func buildPrompt(cfg Config) string {
	persona, ok := council.Personas[cfg.Role]
	if !ok {
		persona = council.Personas["strict-critic"]
	}

	var sb strings.Builder
	sb.WriteString(persona)
	sb.WriteString("\n\n")
	fmt.Fprintf(&sb, "Review the following file: %s\n\n", cfg.Path)
	sb.WriteString("```\n")
	sb.WriteString(cfg.File)
	sb.WriteString("\n```\n\n")
	sb.WriteString("Output template — fill in every section exactly, do not add or remove sections:\n\n")
	sb.WriteString(reportTemplate)
	return sb.String()
}
