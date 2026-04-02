// Package testgen generates Go test stubs for source code files or diffs.
package testgen

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed templates/tests.md
var testsTemplate string

// Runner is the port for LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds inputs for a testgen run.
// File mode: set File and Path.
// Diff mode: set Diff (Log optional).
// Exactly one mode must be active; Run returns an error otherwise.
type Config struct {
	// file mode
	File string
	Path string

	// diff mode
	Diff string
	Log  string

	Runner Runner
}

// Run generates Go test stubs for the configured input using the runner.
func Run(ctx context.Context, cfg Config) (string, error) {
	if cfg.File != "" && cfg.Diff != "" {
		return "", fmt.Errorf("testgen: set File or Diff, not both")
	}
	if cfg.File == "" && cfg.Diff == "" {
		return "", fmt.Errorf("testgen: one of File or Diff must be set")
	}
	return cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
}

func buildPrompt(cfg Config) string {
	var sb strings.Builder
	if cfg.File != "" {
		sb.WriteString("You are generating Go test stubs for a software engineer.\n\n")
		fmt.Fprintf(&sb, "File: %s\n\n", cfg.Path)
		sb.WriteString("Source:\n```go\n")
		sb.WriteString(cfg.File)
		sb.WriteString("\n```\n\n")
		sb.WriteString("Find existing *_test.go files in the same package to match style conventions.\n\n")
	} else {
		sb.WriteString("You are generating Go test stubs covering new or changed behavior.\n\n")
		fmt.Fprintf(&sb, "Commits:\n%s\n\n", ifEmpty(cfg.Log, "(none)"))
		sb.WriteString("Diff:\n```\n")
		sb.WriteString(cfg.Diff)
		sb.WriteString("\n```\n\n")
		sb.WriteString("Identify new and changed exported functions/methods and generate tests for each.\n\n")
	}
	sb.WriteString("Output ONLY valid Go test code. No prose or markdown outside code blocks.\n\n")
	sb.WriteString("Follow this template:\n\n")
	sb.WriteString(testsTemplate)
	return sb.String()
}

func ifEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
