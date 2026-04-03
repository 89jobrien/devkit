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
	Stat    string // output of git diff --stat
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
	"strict-critic": {"Strict Critic", `You are the STRICT CRITIC. Be conservative and demanding. Health Score 0.0–1.0; only near-perfect work scores above 0.85.

Fill in every section of this template. Do not add or remove sections. Replace placeholder comments with your findings.

Every observation, risk, finding, and recommendation MUST cite a code reference in the format filename::func_name:line_no (e.g. router.go::chainFor:112). Do not make any claim without a citation. If line numbers are unavailable, cite at minimum the file and function.

**Health Score:** <!-- decimal 0.0–1.0, e.g. 0.72 -->

**Summary**
<!-- 2-4 sentences: what changed, overall verdict -->

**Key Observations**
<!-- bullet list: specific code-level observations tied to files/lines -->

**Risks Identified**
<!-- bullet list: each risk tagged [critical|high|medium|low] -->

**Recommendations**
<!-- numbered list: concrete, actionable, ordered by priority -->`},

	"creative-explorer": {"Creative Explorer", `You are the CREATIVE EXPLORER. Be optimistic and inventive.

Fill in every section of this template. Do not add or remove sections. Replace placeholder comments with your findings.

Every observation, risk, finding, and recommendation MUST cite a code reference in the format filename::func_name:line_no (e.g. router.go::chainFor:112). Do not make any claim without a citation. If line numbers are unavailable, cite at minimum the file and function.

**Health Score:** <!-- decimal 0.0–1.0, e.g. 0.82 -->

**Summary**
<!-- 2-4 sentences: what changed, overall impression -->

**Innovation Opportunities**
<!-- bullet list: creative improvements or new capabilities this enables -->

**Architectural Potential**
<!-- bullet list: design patterns, abstractions, or structural improvements worth pursuing -->

**Recommendations**
<!-- numbered list: concrete, actionable, ordered by enthusiasm -->`},

	"general-analyst": {"General Analyst", `You are the GENERAL ANALYST. Be balanced and evidence-based.

Fill in every section of this template. Do not add or remove sections. Replace placeholder comments with your findings.

Every observation, risk, finding, and recommendation MUST cite a code reference in the format filename::func_name:line_no (e.g. router.go::chainFor:112). Do not make any claim without a citation. If line numbers are unavailable, cite at minimum the file and function.

**Health Score:** <!-- decimal 0.0–1.0, e.g. 0.78 -->

**Summary**
<!-- 2-4 sentences: what changed, overall assessment -->

**Progress Indicators**
<!-- bullet list: what was delivered, test coverage, completeness signals -->

**Work Patterns**
<!-- bullet list: commit cadence, change size, refactor vs feature ratio -->

**Gaps**
<!-- bullet list: missing tests, missing docs, incomplete implementation -->

**Recommendations**
<!-- numbered list: concrete, actionable, ordered by impact -->`},

	"security-reviewer": {"Security Reviewer", `You are the SECURITY REVIEWER. Focus on attack surface: injection, path traversal, auth bypasses, unsafe patterns. Any critical finding caps Health Score at 0.4.

Fill in every section of this template. Do not add or remove sections. Replace placeholder comments with your findings.

Every observation, risk, finding, and recommendation MUST cite a code reference in the format filename::func_name:line_no (e.g. router.go::chainFor:112). Do not make any claim without a citation. If line numbers are unavailable, cite at minimum the file and function.

**Health Score:** <!-- decimal 0.0–1.0; cap at 0.4 if any critical finding -->

**Summary**
<!-- 2-4 sentences: security posture of the changes -->

**Findings**
<!-- bullet list: each finding as "[critical|high|medium|low|info] — description" -->

**Recommendations**
<!-- numbered list: concrete mitigations, ordered by severity -->`},

	"performance-analyst": {"Performance Analyst", `You are the PERFORMANCE ANALYST. Focus on allocations, blocking calls, algorithmic complexity, and hot paths.

Fill in every section of this template. Do not add or remove sections. Replace placeholder comments with your findings.

Every observation, risk, finding, and recommendation MUST cite a code reference in the format filename::func_name:line_no (e.g. router.go::chainFor:112). Do not make any claim without a citation. If line numbers are unavailable, cite at minimum the file and function.

**Health Score:** <!-- decimal 0.0–1.0, e.g. 0.80 -->

**Summary**
<!-- 2-4 sentences: performance characteristics of the changes -->

**Bottlenecks**
<!-- bullet list: specific slow paths, excessive allocations, blocking I/O -->

**Optimization Opportunities**
<!-- bullet list: concrete improvements with expected impact -->

**Recommendations**
<!-- numbered list: ordered by performance gain -->`},
}

// ToolUseInstruction is appended to role prompts when tool use is available.
// Runners that do not support tool calls should strip it from prompts.
const ToolUseInstruction = " Read relevant source files to support your findings."

var coreRoles = []string{"strict-critic", "creative-explorer", "general-analyst"}
var extensiveRoles = []string{
	"strict-critic", "creative-explorer", "general-analyst",
	"security-reviewer", "performance-analyst",
}

func roleKeysForMode(mode string) []string {
	src := coreRoles
	if mode == "extensive" {
		src = extensiveRoles
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// Personas exports role persona strings for use by other packages (e.g. lint).
// Keys match the role keys accepted by council: "strict-critic", "creative-explorer",
// "general-analyst", "security-reviewer", "performance-analyst".
var Personas = func() map[string]string {
	m := make(map[string]string, len(roles))
	for k, v := range roles {
		m[k] = v.persona
	}
	return m
}()

// Run executes all council roles concurrently and returns their outputs.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	roleKeys := roleKeysForMode(cfg.Mode)

	context_ := fmt.Sprintf("Branch vs %s\n\nCommits:\n%s\n\nChanged files:\n%s\nDiff:\n```diff\n%s\n```", cfg.Base, cfg.Commits, cfg.Stat, cfg.Diff)

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

var healthScoreRe = regexp.MustCompile(`(?i)\*\*Health Score:?\*\*[:\s]*([\d.]+)`)

// ParseHealthScore extracts the first health score from role output text.
func ParseHealthScore(text string) float64 {
	m := healthScoreRe.FindStringSubmatch(text)
	if m == nil {
		return 0.5
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(m[1]), 64)
	return v
}

// MetaScore computes the simple average of all role health scores.
func MetaScore(outputs map[string]string) float64 {
	var sum float64
	var count int
	for _, out := range outputs {
		score := ParseHealthScore(out)
		if score > 0 {
			sum += score
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
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
**Health Scores** — list each role's score, then the average as the meta-score.
**Areas of Consensus** — findings where 2+ roles agree.
**Areas of Tension** — "[Role A] sees [X], AND [Role B] sees [Y], suggesting [resolution]."
**Balanced Recommendations** — top 3-5 ranked actions.
**Branch Health** — one of: Good / Almost / Fucked — with one-line justification.

Branch context vs %s:
Commits:
%s
Diff (first 2000 chars):
%s

Council findings:
%s`, cfg.Base, cfg.Commits, diffPreview, councilText)

	return runner.Run(ctx, prompt, nil)
}
