// internal/docgen/docgen.go
package docgen

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

// Config holds all inputs for a docgen run.
type Config struct {
	File   string // file content
	Path   string // original file path
	Runner Runner
}

// Run generates GoDoc-style documentation for the provided file.
func Run(ctx context.Context, cfg Config) (string, error) {
	return cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
}

func buildPrompt(cfg Config) string {
	var sb strings.Builder
	sb.WriteString("You are an expert Go developer. Generate GoDoc-style documentation comments for the following Go source file.\n\n")
	fmt.Fprintf(&sb, "File: %s\n\n", cfg.Path)
	sb.WriteString("```go\n")
	sb.WriteString(cfg.File)
	sb.WriteString("\n```\n\n")
	sb.WriteString("Output ONLY the documentation comments — the `// Package ...` comment at the top and `//` doc comments for each exported function, type, method, and constant. ")
	sb.WriteString("Do NOT output the full file. Follow GoDoc conventions: first sentence is a summary starting with the symbol name. Be concise and accurate.")
	return sb.String()
}
