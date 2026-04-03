// internal/profile/profile.go
package profile

import (
	"context"
	"fmt"
	"strings"
)

const maxInputBytes = 30 * 1024 // 30KB

// Runner is the port for LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds all inputs for a profile run.
type Config struct {
	Input  string // pprof text output or benchmark output (capped at 30KB)
	Runner Runner
}

// Run analyzes pprof or benchmark output with LLM commentary.
func Run(ctx context.Context, cfg Config) (string, error) {
	if cfg.Runner == nil {
		return "", fmt.Errorf("profile: runner is required")
	}
	return cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
}

func buildPrompt(cfg Config) string {
	input := cfg.Input
	if len(input) > maxInputBytes {
		input = input[:maxInputBytes] + "\n[truncated]"
	}

	var sb strings.Builder
	sb.WriteString("You are a Go performance engineer. Analyze the following pprof profile output or Go benchmark results.\n\n")
	sb.WriteString("Provide:\n")
	sb.WriteString("1. **Hotspots** — top CPU consumers or allocators, explained in plain English\n")
	sb.WriteString("2. **Top Allocators** — highest memory allocation sites (if present)\n")
	sb.WriteString("3. **Benchmark Regressions** — any benchmarks showing slowdowns (if benchmark output)\n")
	sb.WriteString("4. **Optimization Suggestions** — specific, actionable recommendations for each hotspot\n\n")
	sb.WriteString("Be concrete: reference function names from the profile. Prioritize the highest-impact changes.\n\n")
	sb.WriteString("### Profile / Benchmark Output\n```\n")
	sb.WriteString(input)
	sb.WriteString("\n```")
	return sb.String()
}
