// Package citriage diagnoses CI failure logs via BAML structured output.
package citriage

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/89jobrien/devkit/internal/repocontext"
)

const maxLogBytes = 64 * 1024 // 64KB

// Runner is the port for the BAML triage call.
type Runner interface {
	Run(ctx context.Context, log, repoContext string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, log, repoContext string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, log, repoContext string) (string, error) {
	return f(ctx, log, repoContext)
}

// Config holds inputs for a ci-triage run.
type Config struct {
	RepoPath string
	RunID    string // optional: gh run id to fetch
	Log      string // optional: pre-loaded log (e.g. from stdin)
	Runner   Runner
}

// Run fetches the CI failure log (if needed) and calls the runner for triage.
func Run(ctx context.Context, cfg Config) (string, error) {
	if cfg.Runner == nil {
		return "", fmt.Errorf("citriage: runner is required")
	}

	rc, err := repocontext.Gather(cfg.RepoPath)
	if err != nil {
		return "", fmt.Errorf("citriage: %w", err)
	}

	log := cfg.Log
	if log == "" {
		log, err = fetchLog(rc.RepoPath, cfg.RunID)
		if err != nil {
			return "", err
		}
	}

	// Cap log at maxLogBytes
	if len(log) > maxLogBytes {
		log = log[:maxLogBytes] + "\n[truncated]"
	}

	return cfg.Runner.Run(ctx, log, rc.Summary())
}

// fetchLog shells out to gh to get the failure log for the given run ID,
// or attempts to find the most recent failed run if runID is empty.
func fetchLog(repoPath, runID string) (string, error) {
	if runID == "" {
		// Find most recent failed run
		out, err := exec.Command("gh", "run", "list", "--status", "failure", "--limit", "1", "--json", "databaseId", "-q", ".[0].databaseId").Output()
		if err != nil {
			return "", fmt.Errorf("citriage: gh run list: %w", err)
		}
		runID = strings.TrimSpace(string(out))
		if runID == "" {
			return "", fmt.Errorf("citriage: no failed runs found")
		}
	}

	cmd := exec.Command("gh", "run", "view", runID, "--log-failed")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("citriage: gh run view %s: %w", runID, err)
	}
	return string(out), nil
}
