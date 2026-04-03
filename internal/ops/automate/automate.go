// Package automate orchestrates routine maintenance tasks against a repo.
package automate

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/89jobrien/devkit/internal/dev/ticket"
	"github.com/89jobrien/devkit/internal/ops/changelog"
	"github.com/89jobrien/devkit/internal/ops/standup"
)

// Runner is the port for LLM calls passed through to delegated packages.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds inputs for an automate run.
type Config struct {
	Tasks    []string // task names from the registry
	RepoPath string
	Runner   Runner
}

// taskFn is the signature for a registered task handler.
type taskFn func(ctx context.Context, repoPath string, r Runner) (string, error)

// registry maps task names to their handler functions.
// Add new tasks here without touching Run.
var registry = map[string]taskFn{
	"changelog": func(ctx context.Context, _ string, r Runner) (string, error) {
		return changelog.Run(ctx, changelog.Config{
			Runner: changelog.RunnerFunc(r.Run),
		})
	},
	"standup": func(ctx context.Context, repoPath string, r Runner) (string, error) {
		return standup.Run(ctx, standup.Config{
			Repos:  []string{repoPath},
			Since:  24 * time.Hour,
			Runner: standup.RunnerFunc(r.Run),
		})
	},
	"tickets": func(ctx context.Context, repoPath string, r Runner) (string, error) {
		return ticket.Run(ctx, ticket.Config{
			Prompt: "Scan the repository for TODO and FIXME comments and create a structured ticket for each one found.",
			Path:   repoPath,
			Runner: ticket.RunnerFunc(r.Run),
		})
	},
}

// Run executes the requested maintenance tasks in sequence.
func Run(ctx context.Context, cfg Config) (string, error) {
	if cfg.Runner == nil {
		return "", fmt.Errorf("automate: runner is required")
	}

	repoPath := cfg.RepoPath
	if repoPath == "" {
		var err error
		repoPath, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("automate: getwd: %w", err)
		}
	}

	var sb strings.Builder
	for _, task := range cfg.Tasks {
		name := strings.TrimSpace(task)
		fn, ok := registry[name]
		if !ok {
			fmt.Fprintf(&sb, "## %s\n\nunknown task: %q\n\n", name, name)
			continue
		}
		heading := strings.ToUpper(name[:1]) + name[1:]
		fmt.Fprintf(&sb, "## %s\n\n", heading)
		result, err := fn(ctx, repoPath, cfg.Runner)
		if err != nil {
			fmt.Fprintf(&sb, "error: %v\n\n", err)
		} else {
			sb.WriteString(result)
			sb.WriteString("\n\n")
		}
	}
	return sb.String(), nil
}
