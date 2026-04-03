// internal/logpattern/logpattern.go
package logpattern

import (
	"context"
	"strings"
)

const maxLogBytes = 50 * 1024 // 50KB

// Runner is the port for LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds all inputs for a log-pattern run.
type Config struct {
	Logs   string // log content (capped at 50KB)
	Runner Runner
}

// Run analyzes logs for recurring error patterns.
func Run(ctx context.Context, cfg Config) (string, error) {
	return cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
}

func buildPrompt(cfg Config) string {
	logs := cfg.Logs
	if len(logs) > maxLogBytes {
		logs = logs[:maxLogBytes] + "\n[truncated]"
	}

	var sb strings.Builder
	sb.WriteString("You are an experienced site reliability engineer. Analyze the following log output for recurring error patterns.\n\n")
	sb.WriteString("For each distinct error pattern found:\n")
	sb.WriteString("- Group similar messages together\n")
	sb.WriteString("- Show the error type / pattern description\n")
	sb.WriteString("- Show the frequency (count)\n")
	sb.WriteString("- Show the first and last occurrence timestamp (if present)\n")
	sb.WriteString("- Suggest a likely root cause\n\n")
	sb.WriteString("Sort patterns by frequency (most frequent first). If no errors are found, say so.\n\n")
	sb.WriteString("### Log Output\n```\n")
	sb.WriteString(logs)
	sb.WriteString("\n```")
	return sb.String()
}
