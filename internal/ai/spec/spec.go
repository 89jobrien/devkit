// internal/ai/spec/spec.go
package spec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

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

// Config holds parameters for a spec review run.
type Config struct {
	Content         string            // full spec file content
	Path            string            // source path (for display)
	Runner          Runner            // default runner for all six roles
	Runners map[string]Runner // per-role overrides
}

// Result holds all role outputs.
type Result struct {
	RoleOutputs map[string]string
}

var roles = map[string]struct{ label, persona string }{
	"completeness": {"Completeness Checker", `You are the COMPLETENESS CHECKER reviewing a software spec document.

Fill in every section of this template. Do not add or remove sections. Replace placeholder
comments with your findings. Cite the spec by section heading (e.g. ## Design) or line number.

**Health Score:** <!-- decimal 0.0–1.0; deduct for each missing section, TBD, or empty placeholder -->

**Summary**
<!-- 2-4 sentences: overall completeness assessment -->

**Missing Sections**
<!-- bullet list: required sections that are absent or empty -->

**Placeholders Found**
<!-- bullet list: TBDs, TODO, "fill in later", vague statements -->

**Recommendations**
<!-- numbered list: concrete additions ordered by priority -->`},

	"ambiguity": {"Ambiguity Detector", `You are the AMBIGUITY DETECTOR reviewing a software spec document.

Fill in every section of this template. Do not add or remove sections. Replace placeholder
comments with your findings. Cite the spec by section heading (e.g. ## Design) or line number.

**Health Score:** <!-- decimal 0.0–1.0; deduct for each requirement that can be read two ways -->

**Summary**
<!-- 2-4 sentences: overall ambiguity assessment -->

**Ambiguous Requirements**
<!-- bullet list: each item as "## Section — [requirement] could mean X or Y" -->

**Unstated Assumptions**
<!-- bullet list: things the spec assumes without stating explicitly -->

**Recommendations**
<!-- numbered list: concrete clarifications ordered by impact -->`},

	"scope": {"Scope Auditor", `You are the SCOPE AUDITOR reviewing a software spec document.

Fill in every section of this template. Do not add or remove sections. Replace placeholder
comments with your findings. Cite the spec by section heading (e.g. ## Design) or line number.

**Health Score:** <!-- decimal 0.0–1.0; deduct for scope creep, over-engineering, or missing decomposition -->

**Summary**
<!-- 2-4 sentences: overall scope assessment -->

**Scope Creep**
<!-- bullet list: features or requirements beyond the stated goal -->

**Over-Engineering Signals**
<!-- bullet list: complexity that is not justified by the requirements -->

**Decomposition Gaps**
<!-- bullet list: areas that should be separate sub-projects or phases -->

**Recommendations**
<!-- numbered list: concrete scope adjustments ordered by priority -->`},

	"critic": {"Strict Critic", `You are the STRICT CRITIC reviewing a software spec document. Be conservative and demanding.
Any critical gap caps Health Score at 0.4.

Fill in every section of this template. Do not add or remove sections. Replace placeholder
comments with your findings. Cite the spec by section heading (e.g. ## Design) or line number.

**Health Score:** <!-- decimal 0.0–1.0; cap at 0.4 if any critical finding -->

**Summary**
<!-- 2-4 sentences: overall verdict -->

**Key Observations**
<!-- bullet list: specific observations tied to spec sections -->

**Risks Identified**
<!-- bullet list: each risk tagged [critical|high|medium|low] -->

**Recommendations**
<!-- numbered list: concrete, actionable, ordered by priority -->`},

	"creative": {"Creative", `You are the CREATIVE reviewer reviewing a software spec document. Be optimistic and inventive.

Fill in every section of this template. Do not add or remove sections. Replace placeholder
comments with your findings. Cite the spec by section heading (e.g. ## Design) or line number.

**Health Score:** <!-- decimal 0.0–1.0 -->

**Summary**
<!-- 2-4 sentences: overall impression -->

**Innovation Opportunities**
<!-- bullet list: creative improvements or extensions the spec enables -->

**Architectural Potential**
<!-- bullet list: design patterns or abstractions worth pursuing -->

**Recommendations**
<!-- numbered list: concrete, actionable, ordered by enthusiasm -->`},

	"generalist": {"Generalist", `You are the GENERALIST reviewing a software spec document. Be balanced and evidence-based.

Fill in every section of this template. Do not add or remove sections. Replace placeholder
comments with your findings. Cite the spec by section heading (e.g. ## Design) or line number.

**Health Score:** <!-- decimal 0.0–1.0 -->

**Summary**
<!-- 2-4 sentences: overall assessment -->

**Progress Indicators**
<!-- bullet list: what the spec gets right, clear requirements, testability signals -->

**Gaps**
<!-- bullet list: missing tests, missing docs, incomplete areas -->

**Recommendations**
<!-- numbered list: concrete, actionable, ordered by impact -->`},
}

var allRoleKeys = []string{"completeness", "ambiguity", "scope", "critic", "creative", "generalist"}

// Run executes all six spec review roles concurrently.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	context_ := fmt.Sprintf("Spec file: %s\n\n```markdown\n%s\n```", cfg.Path, cfg.Content)

	outputs := make(map[string]string, len(allRoleKeys))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	for _, key := range allRoleKeys {
		key := key
		role := roles[key]
		g.Go(func() error {
			prompt := fmt.Sprintf("%s\n\nReview this spec.\n\n%s", role.persona, context_)
			r := cfg.Runner
			if cfg.Runners != nil {
				if override, ok := cfg.Runners[key]; ok {
					r = override
				}
			}
			if r == nil {
				return fmt.Errorf("role %s: no runner configured", key)
			}
			out, err := r.Run(gctx, prompt, nil)
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

// Synthesize runs a synthesis pass over all role outputs.
func Synthesize(ctx context.Context, outputs map[string]string, path string, runner Runner) (string, error) {
	var parts []string
	for key, out := range outputs {
		parts = append(parts, fmt.Sprintf("### %s\n%s", roles[key].label, out))
	}
	councilText := strings.Join(parts, "\n\n---\n\n")

	prompt := fmt.Sprintf(`Synthesize this multi-role spec review into a final verdict.

Required sections:
**Health Scores** — list each role's score, then the average as the meta-score.
**Areas of Consensus** — findings where 2+ roles agree.
**Areas of Tension** — "[Role A] sees [X], AND [Role B] sees [Y], suggesting [resolution]."
**Balanced Recommendations** — top 3-5 ranked actions.
**Spec Health** — one of: Ready / Almost / Needs Work — with one-line justification.

Spec file: %s

Role findings:
%s`, path, councilText)

	return runner.Run(ctx, prompt, nil)
}

// LatestSpecFile returns the path of the most recently modified .md file in dir.
// Returns an error if dir contains no .md files.
func LatestSpecFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("reading spec dir %s: %w", dir, err)
	}

	var latest string
	var latestMod time.Time

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latestMod) {
			latestMod = info.ModTime()
			latest = filepath.Join(dir, e.Name())
		}
	}

	if latest == "" {
		return "", fmt.Errorf("no .md files found in %s", dir)
	}
	return latest, nil
}
