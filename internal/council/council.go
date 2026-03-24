// internal/council/council.go
package council

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"sync"

	"golang.org/x/sync/errgroup"
)

// Runner is the port for executing LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds parameters for a council run.
type Config struct {
	Base    string
	Mode    string // "core" or "extensive"
	Diff    string
	Commits string
	Runner  Runner
	// Runners overrides Runner for specific role keys (e.g. "creative-explorer").
	// Roles not present here fall back to Runner.
	Runners map[string]Runner
}

// Result holds all role outputs and the synthesis.
type Result struct {
	RoleOutputs map[string]string
	Synthesis   string
}

var roles = map[string]struct{ label, persona string }{
	"strict-critic": {"Strict Critic",
		"You are the STRICT CRITIC. Be conservative and demanding. Health Score 0.0-1.0 (only near-perfect scores above 0.85). Include: **Health Score**, **Summary**, **Key Observations**, **Risks Identified**, **Recommendations**."},
	"creative-explorer": {"Creative Explorer",
		"You are the CREATIVE EXPLORER. Be optimistic and inventive. Include: **Health Score**, **Summary**, **Innovation Opportunities**, **Architectural Potential**, **Recommendations**."},
	"general-analyst": {"General Analyst",
		"You are the GENERAL ANALYST. Be balanced and evidence-based. Include: **Health Score**, **Summary**, **Progress Indicators**, **Work Patterns**, **Gaps**, **Recommendations**."},
	"security-reviewer": {"Security Reviewer",
		"You are the SECURITY REVIEWER. Focus on attack surface: injection, traversal, auth bypasses, unsafe patterns. Include: **Health Score** (any critical vuln = max 0.4), **Summary**, **Findings** (critical/high/medium/low), **Recommendations**."},
	"performance-analyst": {"Performance Analyst",
		"You are the PERFORMANCE ANALYST. Focus on allocations, blocking calls, algorithmic complexity. Include: **Health Score**, **Summary**, **Bottlenecks**, **Optimization Opportunities**, **Recommendations**."},
}

// ToolUseInstruction is appended to role prompts when tool use is available.
// Runners that do not support tool calls should strip it from prompts.
const ToolUseInstruction = " Read relevant source files to support your findings."

var coreRoles = []string{"strict-critic", "creative-explorer", "general-analyst"}
var extensiveRoles = append(append([]string{}, coreRoles...), "security-reviewer", "performance-analyst")

func roleKeysForMode(mode string) []string {
	src := coreRoles
	if mode == "extensive" {
		src = extensiveRoles
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// Run executes all council roles concurrently and returns their outputs.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	roleKeys := roleKeysForMode(cfg.Mode)

	context_ := fmt.Sprintf("Branch vs %s\n\nCommits:\n%s\n\nDiff:\n```diff\n%s\n```", cfg.Base, cfg.Commits, cfg.Diff)

	outputs := make(map[string]string, len(roleKeys))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	for _, key := range roleKeys {
		key := key
		role := roles[key]
		g.Go(func() error {
			prompt := fmt.Sprintf("%s\n\nAnalyse this branch.%s\n\n%s", role.persona, ToolUseInstruction, context_)
			r := cfg.Runner
			if cfg.Runners != nil {
				if override, ok := cfg.Runners[key]; ok {
					r = override
				}
			}
			if r == nil {
				return fmt.Errorf("role %s: no runner configured", key)
			}
			out, err := r.Run(gctx, prompt, []string{"Read", "Glob", "Grep"})
			if err != nil {
				return fmt.Errorf("role %s: %w", key, err)
			}
			mu.Lock()
			outputs[key] = out
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &Result{RoleOutputs: outputs}, nil
}

var healthScoreRe = regexp.MustCompile(`(?i)\*\*Health Score\*\*[:\s]*([\d.]+)`)

// ParseHealthScore extracts the first health score from role output text.
func ParseHealthScore(text string) float64 {
	m := healthScoreRe.FindStringSubmatch(text)
	if m == nil {
		return 0.5
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(m[1]), 64)
	return v
}

// MetaScore computes the weighted meta-score (Strict Critic 1.5x, others 1.0x).
func MetaScore(outputs map[string]string) float64 {
	weights := map[string]float64{
		"strict-critic":       1.5,
		"creative-explorer":   1.0,
		"general-analyst":     1.0,
		"security-reviewer":   1.0,
		"performance-analyst": 1.0,
	}
	var sum, totalW float64
	for key, out := range outputs {
		w := weights[key]
		if w == 0 {
			w = 1.0
		}
		sum += ParseHealthScore(out) * w
		totalW += w
	}
	if totalW == 0 {
		return 0
	}
	return sum / totalW
}

// Synthesize runs a synthesis agent over all role outputs.
func Synthesize(ctx context.Context, outputs map[string]string, cfg Config, runner Runner) (string, error) {
	var parts []string
	for key, out := range outputs {
		parts = append(parts, fmt.Sprintf("### %s\n%s", roles[key].label, out))
	}
	councilText := strings.Join(parts, "\n\n---\n\n")

	diffPreview := cfg.Diff
	if len(diffPreview) > 2000 {
		diffPreview = diffPreview[:2000]
	}

	prompt := fmt.Sprintf(`Synthesize this multi-role council review into a final verdict.

Required sections:
**Health Scores** — list each role's score, compute meta-score (Strict Critic 1.5x weight).
**Areas of Consensus** — findings where 2+ roles agree.
**Areas of Tension** — "[Role A] sees [X], AND [Role B] sees [Y], suggesting [resolution]."
**Balanced Recommendations** — top 3-5 ranked actions.
**Branch Health** — one of: Good / Needs work / Significant issues — with one-line justification.

Branch context vs %s:
Commits:
%s
Diff (first 2000 chars):
%s

Council findings:
%s`, cfg.Base, cfg.Commits, diffPreview, councilText)

	return runner.Run(ctx, prompt, nil)
}
