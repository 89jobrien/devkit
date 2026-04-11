// internal/chain/stages_wiring.go
package chain

import (
	"context"
	"os/exec"
	"strings"

	"github.com/89jobrien/devkit/internal/ai/providers"
	"github.com/89jobrien/devkit/internal/dev/pr"
	"github.com/89jobrien/devkit/internal/dev/ticket"
	"github.com/89jobrien/devkit/internal/ops/citriage"
	"github.com/89jobrien/devkit/internal/ops/diagnose"
	"github.com/89jobrien/devkit/internal/ops/logpattern"
)

// StageWiringConfig holds all configuration needed to build wired stage runners.
type StageWiringConfig struct {
	RepoPath     string
	RunID        string // optional: GHA run ID for ci-triage
	DiffBase     string // optional: base branch/ref for diff-based stages
	AnthropicKey string
	OpenAIKey    string
	GeminiKey    string
	ProviderURLs providers.RouterConfig // URL overrides for testing
}

// BuildStageRunners constructs a map of stage name → StageRunner for all
// canonical stages, wired to the actual devkit command packages.
func BuildStageRunners(cfg StageWiringConfig) map[string]StageRunner {
	router := providers.NewRouter(providers.RouterConfig{
		AnthropicKey: cfg.AnthropicKey,
		OpenAIKey:    cfg.OpenAIKey,
		GeminiKey:    cfg.GeminiKey,
		AnthropicURL: cfg.ProviderURLs.AnthropicURL,
		OpenAIURL:    cfg.ProviderURLs.OpenAIURL,
		GeminiURL:    cfg.ProviderURLs.GeminiURL,
	})

	return map[string]StageRunner{
		"council":     buildCouncilStage(cfg, router),
		"ci-triage":   buildCITriageStage(cfg, router),
		"log-pattern": buildLogPatternStage(router),
		"diagnose":    buildDiagnoseStage(router),
		"ticket":      buildTicketStage(cfg, router),
		"pr":          buildPRStage(cfg, router),
		"meta":        buildMetaStage(router),
	}
}

// priorOutput returns the Output field of the first Result with the given stage name.
func priorOutput(prior []Result, stage string) string {
	for _, r := range prior {
		if r.Stage == stage {
			return r.Output
		}
	}
	return ""
}

// allPriorOutputs concatenates all prior stage outputs into a single string.
func allPriorOutputs(prior []Result) string {
	var sb strings.Builder
	for _, r := range prior {
		if r.Stage != "" && r.Output != "" {
			sb.WriteString("## ")
			sb.WriteString(r.Stage)
			sb.WriteString("\n")
			sb.WriteString(r.Output)
			sb.WriteString("\n\n")
		}
	}
	return sb.String()
}

// gitOutput runs a git command in cfg.RepoPath and returns stdout (empty on error).
func gitOutput(repoPath string, args ...string) string {
	full := append([]string{"-C", repoPath}, args...)
	out, err := exec.Command("git", full...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func buildCouncilStage(cfg StageWiringConfig, router *providers.Router) StageRunner {
	return StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
		base := cfg.DiffBase
		if base == "" {
			base = "main"
		}
		diff := gitOutput(cfg.RepoPath, "diff", base)
		stat := gitOutput(cfg.RepoPath, "diff", "--stat", base)
		commits := gitOutput(cfg.RepoPath, "log", "--oneline", base+"..HEAD")

		// Build per-role runners from the router.
		roleRunners := make(map[string]councilRunner)
		for _, role := range []string{"strict-critic", "creative-explorer", "performance-analyst", "general-analyst", "security-reviewer"} {
			r := router.For(providers.TierForRole(role))
			roleRunners[role] = r
		}

		// Aggregate role outputs manually (council package requires TTY; use simple approach).
		var sb strings.Builder
		healthSum := 0.0
		roleOutputs := make(map[string]string)
		for role, r := range roleRunners {
			prompt := "You are a " + role + ". Review the following diff.\n\nBase: " + base +
				"\n\nCommits:\n" + commits +
				"\n\nChanged files:\n" + stat +
				"\n\nDiff:\n" + diff
			out, err := r.Run(ctx, prompt, nil)
			if err != nil {
				out = "(error: " + err.Error() + ")"
			}
			roleOutputs[role] = out
			sb.WriteString("### ")
			sb.WriteString(role)
			sb.WriteString("\n")
			sb.WriteString(out)
			sb.WriteString("\n\n")
			// Extract health score if present (best-effort).
			healthSum += extractHealthScore(out)
		}

		avgHealth := 0.0
		if len(roleRunners) > 0 {
			avgHealth = healthSum / float64(len(roleRunners))
		}

		return Result{
			Stage:   "council",
			Output:  sb.String(),
			Payload: &CouncilPayload{HealthScore: avgHealth, RoleOutputs: roleOutputs},
		}
	})
}

// councilRunner is a local alias to avoid importing the council package.
type councilRunner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// extractHealthScore does a best-effort parse of "Health Score: 0.72" from text.
func extractHealthScore(s string) float64 {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, "health score")
	if idx < 0 {
		return 0.5 // neutral default
	}
	fragment := s[idx:]
	// Find the first digit sequence after the label.
	start := -1
	for i, c := range fragment {
		if (c >= '0' && c <= '9') || c == '.' {
			start = i
			break
		}
	}
	if start < 0 {
		return 0.5
	}
	end := start
	for end < len(fragment) {
		c := fragment[end]
		if (c >= '0' && c <= '9') || c == '.' {
			end++
		} else {
			break
		}
	}
	var score float64
	_, err := strings.NewReader(fragment[start:end]).Read(nil) // parse below
	_ = err
	// Simple manual parse to avoid importing strconv indirectly causing issues.
	val := fragment[start:end]
	score = parseSimpleFloat(val)
	return score
}

func parseSimpleFloat(s string) float64 {
	// Parse a simple decimal like "0.72" without strconv to avoid import cycles.
	// Falls back to 0.5 on any error.
	if s == "" {
		return 0.5
	}
	var intPart, fracPart int64
	var fracDiv float64 = 1
	dotSeen := false
	for _, c := range s {
		if c == '.' {
			dotSeen = true
			continue
		}
		if c < '0' || c > '9' {
			break
		}
		d := int64(c - '0')
		if dotSeen {
			fracPart = fracPart*10 + d
			fracDiv *= 10
		} else {
			intPart = intPart*10 + d
		}
	}
	return float64(intPart) + float64(fracPart)/fracDiv
}

func buildCITriageStage(cfg StageWiringConfig, router *providers.Router) StageRunner {
	return StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
		r := citriage.RunnerFunc(func(ctx context.Context, log, repoContext string) (string, error) {
			return router.For(providers.TierBalanced).Run(ctx, log+"\n\nRepo context:\n"+repoContext, nil)
		})
		out, err := citriage.Run(ctx, citriage.Config{
			RepoPath: cfg.RepoPath,
			RunID:    cfg.RunID,
			Runner:   r,
		})
		if err != nil {
			return Result{Stage: "ci-triage", Err: err}
		}
		return Result{
			Stage:   "ci-triage",
			Output:  out,
			Payload: &CITriagePayload{RootCause: out},
		}
	})
}

func buildLogPatternStage(router *providers.Router) StageRunner {
	return StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
		logs := priorOutput(prior, "ci-triage")
		if logs == "" {
			logs = priorOutput(prior, "diagnose")
		}
		r := logpattern.RunnerFunc(func(ctx context.Context, prompt string, tools []string) (string, error) {
			return router.For(providers.TierBalanced).Run(ctx, prompt, nil)
		})
		out, err := logpattern.Run(ctx, logpattern.Config{
			Logs:   logs,
			Runner: r,
		})
		if err != nil {
			return Result{Stage: "log-pattern", Err: err}
		}
		return Result{
			Stage:   "log-pattern",
			Output:  out,
			Payload: &LogPatternPayload{Patterns: []string{out}, Count: 1},
		}
	})
}

func buildDiagnoseStage(router *providers.Router) StageRunner {
	return StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
		r := diagnose.RunnerFunc(func(ctx context.Context, prompt string, tools []string) (string, error) {
			return router.For(providers.TierBalanced).Run(ctx, prompt, nil)
		})
		out, err := diagnose.Run(ctx, diagnose.Config{
			Runner: r,
		})
		if err != nil {
			return Result{Stage: "diagnose", Err: err}
		}
		return Result{
			Stage:   "diagnose",
			Output:  out,
			Payload: &DiagnosePayload{Summary: out},
		}
	})
}

func buildTicketStage(cfg StageWiringConfig, router *providers.Router) StageRunner {
	return StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
		prompt := allPriorOutputs(prior)
		if prompt == "" {
			prompt = "Generate a ticket for the changes in this repository."
		}
		r := ticket.RunnerFunc(func(ctx context.Context, p string, tools []string) (string, error) {
			return router.For(providers.TierBalanced).Run(ctx, p, nil)
		})
		out, err := ticket.Run(ctx, ticket.Config{
			Prompt: prompt,
			Path:   cfg.RepoPath,
			Runner: r,
		})
		if err != nil {
			return Result{Stage: "ticket", Err: err}
		}
		return Result{
			Stage:   "ticket",
			Output:  out,
			Payload: &TicketPayload{Title: "Generated ticket", Body: out},
		}
	})
}

func buildPRStage(cfg StageWiringConfig, router *providers.Router) StageRunner {
	return StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
		base := cfg.DiffBase
		if base == "" {
			base = pr.ResolveBase("")
		}
		diff := gitOutput(cfg.RepoPath, "diff", base)
		log := gitOutput(cfg.RepoPath, "log", "--oneline", base+"..HEAD")
		stat := gitOutput(cfg.RepoPath, "diff", "--stat", base)

		r := pr.RunnerFunc(func(ctx context.Context, prompt string, tools []string) (string, error) {
			return router.For(providers.TierBalanced).Run(ctx, prompt, nil)
		})
		out, err := pr.Run(ctx, pr.Config{
			Base:   base,
			Diff:   diff,
			Log:    log,
			Stat:   stat,
			Runner: r,
		})
		if err != nil {
			return Result{Stage: "pr", Err: err}
		}
		return Result{
			Stage:   "pr",
			Output:  out,
			Payload: &PRPayload{Title: "Generated PR", Body: out},
		}
	})
}

func buildMetaStage(router *providers.Router) StageRunner {
	return StageRunnerFunc(func(ctx context.Context, prior []Result) Result {
		combined := allPriorOutputs(prior)
		prompt := "Synthesize the following pipeline stage outputs into a concise executive summary " +
			"with key findings and recommended next actions.\n\n" + combined
		out, err := router.For(providers.TierCoding).Run(ctx, prompt, nil)
		if err != nil {
			return Result{Stage: "meta", Err: err}
		}
		return Result{
			Stage:   "meta",
			Output:  out,
			Payload: &SynthesisPayload{Summary: out},
		}
	})
}
