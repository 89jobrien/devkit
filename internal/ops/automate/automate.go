// Package automate orchestrates routine maintenance tasks against a repo.
package automate

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/89jobrien/devkit/internal/ops/changelog"
	"github.com/89jobrien/devkit/internal/ops/standup"
	"github.com/89jobrien/devkit/internal/dev/ticket"
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
	Tasks    []string // "changelog", "standup", "tickets"
	RepoPath string
	Runner   Runner
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
		switch strings.TrimSpace(task) {
		case "changelog":
			fmt.Fprintf(&sb, "## Changelog\n\n")
			result, err := changelog.Run(ctx, changelog.Config{
				Runner: changelog.RunnerFunc(cfg.Runner.Run),
			})
			if err != nil {
				fmt.Fprintf(&sb, "error: %v\n\n", err)
			} else {
				sb.WriteString(result)
				sb.WriteString("\n\n")
			}

		case "standup":
			fmt.Fprintf(&sb, "## Standup\n\n")
			result, err := standup.Run(ctx, standup.Config{
				Repos: []string{repoPath},
				Since: 24 * time.Hour,
				Runner: standup.RunnerFunc(cfg.Runner.Run),
			})
			if err != nil {
				fmt.Fprintf(&sb, "error: %v\n\n", err)
			} else {
				sb.WriteString(result)
				sb.WriteString("\n\n")
			}

		case "tickets":
			fmt.Fprintf(&sb, "## Tickets\n\n")
			result, err := ticket.Run(ctx, ticket.Config{
				Prompt: "Scan the repository for TODO and FIXME comments and create a structured ticket for each one found.",
				Path:   repoPath,
				Runner: ticket.RunnerFunc(cfg.Runner.Run),
			})
			if err != nil {
				fmt.Fprintf(&sb, "error: %v\n\n", err)
			} else {
				sb.WriteString(result)
				sb.WriteString("\n\n")
			}

		default:
			fmt.Fprintf(&sb, "## %s\n\nunknown task: %q\n\n", task, task)
		}
	}
	return sb.String(), nil
}
