// internal/standup/standup.go
package standup

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
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

// Config holds all inputs for a standup run.
type Config struct {
	Repos    []string      // resolved absolute paths; cwd if empty
	Since    time.Duration // default 24h
	Runner   Runner
	Parallel bool
}

// repoContext holds gathered data for one repository.
type repoContext struct {
	Path    string
	Project string
	Commits string
	Stat    string
	Runs    []runEntry
}

// runEntry is a single JSONL entry from agent-runs.jsonl.
type runEntry struct {
	Command    string            `json:"command"`
	Status     string            `json:"status"`
	DurationMs int64             `json:"duration_ms"`
	Args       map[string]string `json:"args"`
	Timestamp  string            `json:"run_id"`
}

// Run is the public entry point.
func Run(ctx context.Context, cfg Config) (string, error) {
	if len(cfg.Repos) == 0 {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}
		cfg.Repos = []string{wd}
	}
	if cfg.Since == 0 {
		cfg.Since = 24 * time.Hour
	}

	if len(cfg.Repos) == 1 && !cfg.Parallel {
		return runSingle(ctx, cfg.Repos[0], cfg.Since, cfg.Runner)
	}
	return runParallel(ctx, cfg)
}

// runSingle gathers context for one repo and calls the LLM once.
func runSingle(ctx context.Context, repoPath string, since time.Duration, runner Runner) (string, error) {
	rc, err := gatherRepoContext(repoPath, since)
	if err != nil {
		return "", err
	}
	prompt := buildSinglePrompt(rc, since)
	return runner.Run(ctx, prompt, nil)
}

// runParallel spawns one goroutine per repo for per-repo summaries, then synthesizes.
func runParallel(ctx context.Context, cfg Config) (string, error) {
	summaries := make([]string, len(cfg.Repos))

	g, gctx := errgroup.WithContext(ctx)
	for i, repoPath := range cfg.Repos {
		i, repoPath := i, repoPath
		g.Go(func() error {
			rc, err := gatherRepoContext(repoPath, cfg.Since)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", repoPath, err)
				return nil
			}
			prompt := buildSummaryPrompt(rc, cfg.Since)
			out, err := cfg.Runner.Run(gctx, prompt, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: runner failed for %s: %v\n", repoPath, err)
				return nil
			}
			summaries[i] = fmt.Sprintf("### %s\n%s", rc.Project, out)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return "", err
	}

	var nonEmpty []string
	for _, s := range summaries {
		if s != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}
	if len(nonEmpty) == 0 {
		return "", fmt.Errorf("standup: no repo summaries available (all repos failed or had no activity)")
	}
	return synthesize(ctx, nonEmpty, cfg.Runner)
}

// gatherRepoContext collects git log, diff stat, and JSONL entries for a repo.
func gatherRepoContext(repoPath string, since time.Duration) (*repoContext, error) {
	// Validate it's a git repo (handles normal repos and worktrees).
	revParse := exec.Command("git", "rev-parse", "--git-dir")
	revParse.Dir = repoPath
	if err := revParse.Run(); err != nil {
		return nil, fmt.Errorf("%s is not a git repository", repoPath)
	}

	run := func(args ...string) string {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoPath
		out, _ := cmd.Output()
		return string(out)
	}

	sinceStr := fmt.Sprintf("%.0fh ago", since.Hours())
	commits := run("git", "log", "--since="+sinceStr, "--oneline")

	// Diff stat — fall back gracefully if the ref doesn't resolve.
	stat := run("git", "diff", "--stat", fmt.Sprintf("HEAD@{%s}", sinceStr), "HEAD")
	if strings.TrimSpace(stat) == "" {
		stat = run("git", "diff", "--stat", "HEAD")
	}

	project := filepath.Base(repoPath)
	runs := gatherJSONLRuns(project, since)

	return &repoContext{
		Path:    repoPath,
		Project: project,
		Commits: commits,
		Stat:    stat,
		Runs:    runs,
	}, nil
}

// gatherJSONLRuns reads ~/.dev-agents/<project>/agent-runs.jsonl, returns complete entries within window.
func gatherJSONLRuns(project string, since time.Duration) []runEntry {
	home, _ := os.UserHomeDir()
	jsonlPath := filepath.Join(home, ".dev-agents", project, "agent-runs.jsonl")

	f, err := os.Open(jsonlPath)
	if err != nil {
		return nil // normal — file may not exist
	}
	defer f.Close()

	cutoff := time.Now().UTC().Add(-since)
	var entries []runEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e runEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if e.Status != "complete" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, e.Timestamp)
		if err != nil {
			continue
		}
		if t.After(cutoff) {
			entries = append(entries, e)
		}
	}
	return entries
}

func writeRepoBody(sb *strings.Builder, rc *repoContext) {
	fmt.Fprintf(sb, "Recent commits:\n%s\n", ifEmpty(rc.Commits, "(no commits in window)"))
	fmt.Fprintf(sb, "Changed files:\n%s\n", ifEmpty(rc.Stat, "(no diff available)"))
	if len(rc.Runs) > 0 {
		sb.WriteString("Devkit runs:\n")
		for _, r := range rc.Runs {
			fmt.Fprintf(sb, "- %s (%.1fs)\n", r.Command, float64(r.DurationMs)/1000)
		}
		sb.WriteString("\n")
	}
}

func buildSinglePrompt(rc *repoContext, since time.Duration) string {
	var sb strings.Builder
	sb.WriteString("You are generating a standup update for a software engineer.\n\n")
	fmt.Fprintf(&sb, "Project: %s\nTime window: last %.0f hours\n\n", rc.Project, since.Hours())
	writeRepoBody(&sb, rc)
	sb.WriteString(`Produce a standup update with exactly three sections:
## What I did
## What's next
## Blockers

Be concise. Infer "what's next" from incomplete work and commit messages. If no blockers are evident, write "none".`)
	return sb.String()
}

func buildSummaryPrompt(rc *repoContext, since time.Duration) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Summarize work done in the %s repository over the last %.0f hours.\n\n", rc.Project, since.Hours())
	writeRepoBody(&sb, rc)
	sb.WriteString("Write 3-5 concise bullet points summarising what was done. No headers, no fluff.")
	return sb.String()
}

func synthesize(ctx context.Context, summaries []string, runner Runner) (string, error) {
	prompt := fmt.Sprintf(`Synthesize the per-repo summaries below into a single standup update.

%s

Produce a standup update with exactly three sections:
## What I did
## What's next
## Blockers

Be concise. If no blockers are evident, write "none".`, strings.Join(summaries, "\n\n---\n\n"))
	return runner.Run(ctx, prompt, nil)
}

func ifEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
