// Package providers defines abstractions for LLM backends and task routing.
package providers

import (
	"context"

	"github.com/89jobrien/devkit/internal/tools"
)

// Runner executes a prompt and returns a response. Tool names are passed for
// providers that support tool-use routing; non-tool runners may ignore them.
type Runner interface {
	Run(ctx context.Context, prompt string, toolNames []string) (string, error)
}

// RunnerFunc adapts a plain function to the Runner interface.
type RunnerFunc func(ctx context.Context, prompt string, toolNames []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, toolNames []string) (string, error) {
	return f(ctx, prompt, toolNames)
}

// ChatProvider is a single-turn LLM completion with no tool use.
type ChatProvider interface {
	Chat(ctx context.Context, prompt string) (string, error)
}

// AgentProvider supports multi-turn tool-use agentic loops.
type AgentProvider interface {
	ChatProvider
	RunAgent(ctx context.Context, prompt string, ts []tools.Tool) (string, error)
}

// Tier classifies the nature of a task for provider selection.
type Tier string

const (
	TierFast         Tier = "fast"          // cheap, exploratory, mapping
	TierBalanced     Tier = "balanced"      // reasoning, synthesis
	TierLargeContext Tier = "large-context" // full diff/repo ingestion
	TierCoding       Tier = "coding"        // agentic tool use, implementation
)
