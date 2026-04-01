// Package providers defines abstractions for LLM backends and task routing.
package providers

import (
	"context"

	"github.com/89jobrien/devkit/internal/tools"
)

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
