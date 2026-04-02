// internal/explain/explain.go
package explain

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed templates/file.md
var fileTemplate string

//go:embed templates/diff.md
var diffTemplate string

// Runner is the port for LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds inputs for an explain run.
// File mode: set File and Path (Symbol optional).
// Diff mode: set Diff (Log and Stat optional).
// Exactly one mode must be active; Run returns an error otherwise.
type Config struct {
	// file mode
	File   string
	Path   string
	Symbol string

	// diff mode
	Diff string
	Log  string
	Stat string

	Runner Runner
}

// Run explains code in the configured mode using the runner.
func Run(ctx context.Context, cfg Config) (string, error) {
	if cfg.File != "" && cfg.Diff != "" {
		return "", fmt.Errorf("explain: set File or Diff, not both")
	}
	if cfg.File == "" && cfg.Diff == "" {
		return "", fmt.Errorf("explain: one of File or Diff must be set")
	}
	return cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
}

func buildPrompt(cfg Config) string {
	var sb strings.Builder
	if cfg.File != "" {
		sb.WriteString("You are explaining source code to a software engineer.\n\n")
		fmt.Fprintf(&sb, "File: %s\n\n", cfg.Path)
		if cfg.Symbol != "" {
			fmt.Fprintf(&sb, "Focus on: %s\n\n", cfg.Symbol)
		}
		sb.WriteString("Source:\n```\n")
		sb.WriteString(cfg.File)
		sb.WriteString("\n```\n\n")
		sb.WriteString("You may read related files in the repository to follow imports and find call sites.\n\n")
		sb.WriteString("Output template — fill in every section exactly, do not add or remove sections:\n\n")
		sb.WriteString(fileTemplate)
	} else {
		sb.WriteString("You are explaining a set of code changes to a software engineer.\n\n")
		fmt.Fprintf(&sb, "Commits:\n%s\n\n", ifEmpty(cfg.Log, "(none)"))
		fmt.Fprintf(&sb, "Changed files:\n%s\n\n", ifEmpty(cfg.Stat, "(none)"))
		sb.WriteString("Diff:\n```\n")
		sb.WriteString(cfg.Diff)
		sb.WriteString("\n```\n\n")
		sb.WriteString("You may read files in the repository for additional context.\n\n")
		sb.WriteString("Output template — fill in every section exactly, do not add or remove sections:\n\n")
		sb.WriteString(diffTemplate)
	}
	return sb.String()
}

func ifEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
