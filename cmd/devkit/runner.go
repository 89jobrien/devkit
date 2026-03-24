// cmd/devkit/runner.go
package main

import (
	"context"
	"os"

	"github.com/89jobrien/devkit/internal/loop"
	"github.com/89jobrien/devkit/internal/tools"
	"github.com/anthropics/anthropic-sdk-go"
)

// agentRunner adapts loop.RunAgent to the Runner interface used by council, review, and meta.
type agentRunner struct {
	client anthropic.Client
}

func newAgentRunner() *agentRunner {
	return &agentRunner{
		client: anthropic.NewClient(), // reads ANTHROPIC_API_KEY from env
	}
}

func (r *agentRunner) Run(ctx context.Context, prompt string, toolNames []string) (string, error) {
	wd, _ := os.Getwd()
	allTools := []tools.Tool{
		tools.ReadTool(wd),
		tools.GlobTool(wd),
		tools.GrepTool(wd),
	}

	var selected []tools.Tool
	if len(toolNames) == 0 {
		selected = allTools
	} else {
		nameSet := make(map[string]bool, len(toolNames))
		for _, n := range toolNames {
			nameSet[n] = true
		}
		for _, t := range allTools {
			if nameSet[t.Definition.OfTool.Name] {
				selected = append(selected, t)
			}
		}
	}

	return loop.RunAgent(ctx, r.client, prompt, selected)
}
