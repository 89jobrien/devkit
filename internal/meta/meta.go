// internal/meta/meta.go
package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"
)

// Runner is the port for executing LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// AgentSpec is output from the designer agent.
type AgentSpec struct {
	Name   string   `json:"name"`
	Role   string   `json:"role"`
	Prompt string   `json:"prompt"`
	Tools  []string `json:"tools"`
}

// Result holds the design plan, worker outputs, and synthesis.
type Result struct {
	Plan    []AgentSpec
	Outputs map[string]string
	Summary string
}

// Run executes the full meta-agent flow: design -> parallel workers -> synthesis.
func Run(ctx context.Context, task, repoContext, sdkDocs string, runner Runner) (*Result, error) {
	plan, err := design(ctx, task, repoContext, sdkDocs, runner)
	if err != nil {
		return nil, fmt.Errorf("design: %w", err)
	}

	outputs := make(map[string]string, len(plan))
	mu := make(chan struct{}, 1)
	mu <- struct{}{}

	g, gctx := errgroup.WithContext(ctx)
	for _, spec := range plan {
		spec := spec
		g.Go(func() error {
			out, err := runner.Run(gctx, spec.Prompt, spec.Tools)
			if err != nil {
				return fmt.Errorf("agent %s: %w", spec.Name, err)
			}
			<-mu
			outputs[spec.Name] = out
			mu <- struct{}{}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	summary, err := synthesize(ctx, task, outputs, runner)
	if err != nil {
		return nil, fmt.Errorf("synthesis: %w", err)
	}

	return &Result{Plan: plan, Outputs: outputs, Summary: summary}, nil
}

func design(ctx context.Context, task, repoContext, sdkDocs string, runner Runner) ([]AgentSpec, error) {
	rcPreview := repoContext
	if len(rcPreview) > 4000 {
		rcPreview = rcPreview[:4000]
	}
	sdkPreview := sdkDocs
	if len(sdkPreview) > 6000 {
		sdkPreview = sdkPreview[:6000]
	}

	prompt := fmt.Sprintf(`You are a meta-agent designer. Design the smallest set of parallel agents to accomplish this task.

Output ONLY a valid JSON array with schema:
[{"name":"kebab-name","role":"one sentence","prompt":"complete self-contained prompt","tools":["Read","Glob","Grep"]}]

Rules:
- 2-5 agents, each with a distinct non-overlapping concern
- Prompts must be fully self-contained
- Available tools: Read, Glob, Grep (reads); Bash, Write, Edit (modifications)
- Only grant Write/Edit/Bash when genuinely needed
- Do NOT include a synthesis agent

## Task
%s

## Repo context
%s

## SDK docs
%s`, task, rcPreview, sdkPreview)

	raw, err := runner.Run(ctx, prompt, nil)
	if err != nil {
		return nil, err
	}

	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		lines := strings.SplitN(raw, "\n", 2)
		if len(lines) == 2 {
			raw = strings.TrimSuffix(strings.TrimSpace(lines[1]), "```")
		}
	}

	var plan []AgentSpec
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return []AgentSpec{{
			Name: "analyst", Role: "General analysis",
			Prompt: task, Tools: []string{"Read", "Glob", "Grep"},
		}}, nil
	}
	return plan, nil
}

func synthesize(ctx context.Context, task string, outputs map[string]string, runner Runner) (string, error) {
	var parts []string
	for name, out := range outputs {
		parts = append(parts, fmt.Sprintf("### %s\n%s", name, out))
	}

	prompt := fmt.Sprintf(`Synthesize outputs from parallel agents into a coherent report.

Required sections:
**Summary** — 2-3 sentences: what the agents found and overall verdict
**Key Findings** — deduplicated bullets grouped by theme
**Recommended Actions** — ranked list with who/what/why
**Open Questions** — anything unresolved

Original task: %s

Agent outputs:
%s`, task, strings.Join(parts, "\n\n---\n\n"))

	return runner.Run(ctx, prompt, nil)
}
