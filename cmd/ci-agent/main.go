// cmd/ci-agent/main.go
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/89jobrien/devkit/internal/infra/platform"
	"github.com/BurntSushi/toml"
)

type devkitConfig struct {
	Project struct {
		Description string `toml:"description"`
	} `toml:"project"`
	Context struct {
		Files []string `toml:"files"`
	} `toml:"context"`
}

func loadConfig() devkitConfig {
	var cfg devkitConfig
	cfg.Context.Files = []string{"CLAUDE.md", "AGENTS.md", "README.md"}
	_, _ = toml.DecodeFile(".devkit.toml", &cfg)
	return cfg
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "ERROR: required env var %s is not set\n", key)
		os.Exit(1)
	}
	return v
}

func main() {
	ciPlatform := requireEnv("CI_PLATFORM")
	repo := requireEnv("REPO")
	runID := requireEnv("RUN_ID")
	sha := requireEnv("COMMIT_SHA")

	var token string
	switch ciPlatform {
	case "gitea":
		token = requireEnv("CI_AGENT_TOKEN")
	case "github":
		token = requireEnv("GITHUB_TOKEN")
	default:
		fmt.Fprintf(os.Stderr, "ERROR: unknown CI_PLATFORM %q\n", ciPlatform)
		os.Exit(1)
	}

	gitBaseURL := os.Getenv("GITEA_URL")

	ctx := context.Background()
	p, err := platform.New(ciPlatform, repo, runID, sha, token, gitBaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}

	fmt.Printf("=== CI Diagnosis Agent — run %s ===\n", runID)

	logs, err := p.FetchFailedJobLogs(ctx, runID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR fetching job logs:", err)
		os.Exit(1)
	}
	if len(logs) == 0 {
		fmt.Println("No failed jobs found — nothing to diagnose.")
		return
	}

	failedNames := make([]string, len(logs))
	for i, l := range logs {
		failedNames[i] = l.Name
	}
	fmt.Printf("Failed jobs: %v\n", failedNames)

	cfg := loadConfig()

	var sections []string
	for _, l := range logs {
		sections = append(sections, fmt.Sprintf("### Job: %s\n```\n%s\n```", l.Name, l.Log))
	}
	logsText := strings.Join(sections, "\n\n")

	var ctxParts []string
	if cfg.Project.Description != "" {
		ctxParts = append(ctxParts, "**Project:** "+cfg.Project.Description)
	}
	for _, f := range cfg.Context.Files {
		data, err := os.ReadFile(f)
		if err == nil {
			ctxParts = append(ctxParts, fmt.Sprintf("**%s:**\n%s", f, string(data)))
		}
	}

	prompt := fmt.Sprintf("You are a CI expert. The following jobs failed.\n\n%s\n\n%s\n\nAnalyze and provide:\n1. Root cause — what exactly failed and why\n2. Fix — the minimal change needed\n3. Confidence — high/medium/low",
		strings.Join(ctxParts, "\n\n"), logsText)

	_ = p.SetCommitStatus(ctx, "pending", "CI diagnosis in progress...")

	diagnosis, provider, err := askWithFallback(prompt)
	if err != nil {
		fmt.Println("All LLM providers failed:", err)
		diagnosis = fmt.Sprintf("All LLM providers failed.\n\n%s", logsText)
		provider = "none"
	}

	fmt.Printf("\n%s\nDIAGNOSIS (via %s)\n%s\n%s\n", strings.Repeat("=", 60), provider, strings.Repeat("=", 60), diagnosis)

	state := "failure"
	if provider == "none" {
		state = "error"
	}
	firstLine := diagnosis
	if i := strings.Index(diagnosis, "\n"); i > 0 {
		firstLine = diagnosis[:i]
	}
	if len(firstLine) > 120 {
		firstLine = firstLine[:120]
	}
	_ = p.SetCommitStatus(ctx, state, "Diagnosed: "+firstLine)

	_ = p.EnsureLabelExists(ctx)
	existing, found, _ := p.FindIssueForCommit(ctx, sha)
	if found {
		fmt.Printf("Appending to existing issue #%d\n", existing)
		_ = p.AddComment(ctx, existing, diagnosis, provider)
	} else {
		num, err := p.CreateIssue(ctx, sha, diagnosis, provider, failedNames, runID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Warning: could not create issue:", err)
		} else {
			fmt.Printf("Opened issue #%d\n", num)
		}
	}
}
