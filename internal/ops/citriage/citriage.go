// Package citriage diagnoses CI failure logs via BAML structured output.
package citriage

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/89jobrien/devkit/internal/repocontext"
)

const maxLogBytes = 64 * 1024 // 64KB

// ghBinary is the path to the gh CLI binary. Overridable in tests via SetGhBinary.
var ghBinary = "gh"

// SetGhBinary overrides the gh binary path used by fetchLog and returns a
// restore function that resets it to the previous value. Intended for tests only.
func SetGhBinary(path string) func() {
	prev := ghBinary
	ghBinary = path
	return func() { ghBinary = prev }
}

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

	log = filterLog(log)

	// Cap log at maxLogBytes
	if len(log) > maxLogBytes {
		log = log[:maxLogBytes] + "\n[truncated]"
	}

	return cfg.Runner.Run(ctx, log, rc.Summary())
}

// Compiled once at package init for filterLog.
var (
	// reANSI matches ANSI escape sequences.
	reANSI = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	// reTimestamp matches ISO-8601 timestamps optionally preceded by a BOM.
	reTimestamp = regexp.MustCompile(`[\xef\xbb\xbf]*\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z `)
	// reJobPrefix matches "<job>\t<step>\t" prefixes emitted by gh run view --log-failed.
	reJobPrefix = regexp.MustCompile(`^[^\t]+\t[^\t]*\t`)
)

// boilerplatePatterns are substrings that identify GHA runner setup noise.
// Lines containing any of these are dropped wholesale.
var boilerplatePatterns = []string{
	"##[group]",
	"##[endgroup]",
	"##[debug]",
	"Runner Image Provisioner",
	"Hosted Compute Agent",
	"Azure Region",
	"Worker ID",
	"Runner Image",
	"Included Software:",
	"Image Release:",
	"GITHUB_TOKEN Permissions",
	"Secret source:",
	"Prepare workflow directory",
	"Prepare all required actions",
	"Getting action download info",
	"Download action repository",
	"Complete job name:",
	"Temporarily overriding HOME=",
	"Adding repository directory to the temporary git global config",
	"Deleting the contents of",
	"Initializing the repository",
	"Disabling automatic garbage collection",
	"Setting up auth",
	"hint: Using 'master' as the name",
	"hint: will change to",
	"hint: to use in all of your new repositories",
	"hint: call:",
	"hint: \tgit config",
	"hint: Names commonly chosen",
	"hint: \tgit branch",
	"hint: Disable this message",
	"Fetching the repository",
	"Determining the checkout info",
	"Checking out the ref",
	"[command]/usr/bin/git config",
	"[command]/usr/bin/git submodule",
	"[command]/usr/bin/git remote",
	"[command]/usr/bin/git init",
	"[command]/usr/bin/git version",
	"[command]/usr/bin/git -c",
	"[command]/usr/bin/git fetch",
	"[command]/usr/bin/git checkout",
}

// filterLog strips timestamps, ANSI codes, job/step prefixes, and GHA runner
// boilerplate to reduce token count before sending to the LLM.
func filterLog(raw string) string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	prevBlank := false
	for _, line := range lines {
		// Strip ANSI codes and timestamps first.
		line = reANSI.ReplaceAllString(line, "")
		line = reTimestamp.ReplaceAllString(line, "")
		// Strip "<job>\t<step>\t" prefix.
		line = reJobPrefix.ReplaceAllString(line, "")
		line = strings.TrimRight(line, " \t\r")

		// Drop boilerplate.
		drop := false
		for _, pat := range boilerplatePatterns {
			if strings.Contains(line, pat) {
				drop = true
				break
			}
		}
		if drop {
			continue
		}

		// Collapse consecutive blank lines.
		blank := line == ""
		if blank && prevBlank {
			continue
		}
		prevBlank = blank
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// fetchLog shells out to gh to get the failure log for the given run ID,
// or attempts to find the most recent failed run if runID is empty.
func fetchLog(repoPath, runID string) (string, error) {
	if runID == "" {
		// Find most recent failed run — cmd.Dir ensures we query the correct repo.
		cmd := exec.Command(ghBinary, "run", "list", "--status", "failure", "--limit", "1", "--json", "databaseId", "-q", ".[0].databaseId")
		cmd.Dir = repoPath
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("citriage: gh run list: %w", err)
		}
		runID = strings.TrimSpace(string(out))
		if runID == "" {
			return "", fmt.Errorf("citriage: no failed runs found")
		}
	}

	cmd := exec.Command(ghBinary, "run", "view", runID, "--log-failed")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("citriage: gh run view %s: %w", runID, err)
	}
	return string(out), nil
}
