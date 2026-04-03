// internal/pr/pr.go
package pr

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Runner is the port for LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds all inputs for a pr run.
type Config struct {
	Base   string // resolved base branch (never empty by the time Run is called)
	Diff   string
	Log    string
	Stat   string
	Runner Runner
}

const maxDiffBytes = 8000

// Run builds a prompt from git context and calls the runner once.
func Run(ctx context.Context, cfg Config) (string, error) {
	prompt := buildPrompt(cfg)
	return cfg.Runner.Run(ctx, prompt, nil)
}

// ResolveBase returns the base branch to diff against.
// If explicit is non-empty it is returned unchanged.
// Otherwise gh repo view is used; on any failure "main" is returned.
func ResolveBase(explicit string) string {
	if explicit != "" {
		return explicit
	}
	out, err := exec.Command("gh", "repo", "view", "--json", "defaultBranch", "--jq", ".defaultBranch").Output()
	if err != nil {
		return "main"
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "main"
	}
	return branch
}

func buildPrompt(cfg Config) string {
	diff := cfg.Diff
	truncated := false
	if len(diff) > maxDiffBytes {
		diff = diff[:maxDiffBytes]
		// trim back to a valid UTF-8 rune boundary
		for len(diff) > 0 && diff[len(diff)-1]&0xC0 == 0x80 {
			diff = diff[:len(diff)-1]
		}
		truncated = true
	}

	var sb strings.Builder
	sb.WriteString("You are generating a pull request description for a software engineer.\n\n")
	fmt.Fprintf(&sb, "Base branch: %s\n\n", cfg.Base)
	fmt.Fprintf(&sb, "Recent commits:\n%s\n", ifEmpty(cfg.Log, "(no commits in range)"))
	fmt.Fprintf(&sb, "Changed files:\n%s\n", ifEmpty(cfg.Stat, "(no changes)"))
	sb.WriteString("Diff:\n")
	sb.WriteString(diff)
	if truncated {
		sb.WriteString("\n[diff truncated]\n")
	}
	sb.WriteString("\nProduce a PR description with title, summary, list of changes, and test plan.")
	return sb.String()
}

func ifEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
