// Package health gathers structured repo health checks and scores them via BAML.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/89jobrien/devkit/internal/repocontext"
)

// Runner is the port for the BAML health scoring call.
type Runner interface {
	Run(ctx context.Context, repoContext, checkResults string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, repoContext, checkResults string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, repoContext, checkResults string) (string, error) {
	return f(ctx, repoContext, checkResults)
}

// Config holds inputs for a health run.
type Config struct {
	RepoPath string
	Runner   Runner
	Format   string // "markdown" (default) or "json"
}

// CheckResult is a single local check outcome.
type CheckResult struct {
	Name     string
	Status   string // pass | warn | fail
	Severity string // info | warning | critical
	Detail   string
}

// Run gathers local checks, then calls the runner for scoring and summary.
func Run(ctx context.Context, cfg Config) (string, error) {
	if cfg.Runner == nil {
		return "", fmt.Errorf("health: runner is required")
	}

	rc, err := repocontext.Gather(cfg.RepoPath)
	if err != nil {
		return "", fmt.Errorf("health: %w", err)
	}

	checks := gatherChecks(rc.RepoPath)
	checkStr := formatChecks(checks)
	output, err := cfg.Runner.Run(ctx, rc.Summary(), checkStr)
	if err != nil {
		return "", err
	}
	if cfg.Format == "json" {
		b, jerr := json.Marshal(map[string]string{"output": output})
		if jerr != nil {
			return "", fmt.Errorf("health: json marshal: %w", jerr)
		}
		return string(b), nil
	}
	return output, nil
}

func gatherChecks(repoPath string) []CheckResult {
	var results []CheckResult

	// CLAUDE.md present
	results = append(results, fileCheck(repoPath, "CLAUDE.md", "CLAUDE.md present", "warning"))

	// CI config present
	ciPresent := dirExists(filepath.Join(repoPath, ".github", "workflows")) ||
		dirExists(filepath.Join(repoPath, ".gitea", "workflows"))
	if ciPresent {
		results = append(results, newCheck("CI config", "pass", "warning", "CI workflow directory found"))
	} else {
		results = append(results, newCheck("CI config", "warn", "warning", "no .github/workflows or .gitea/workflows found"))
	}

	// Test files present
	testFiles := countFiles(repoPath, func(name string) bool {
		return strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_test.rs")
	})
	if testFiles > 0 {
		results = append(results, newCheck("Test files", "pass", "warning", fmt.Sprintf("%d test file(s) found", testFiles)))
	} else {
		results = append(results, newCheck("Test files", "warn", "warning", "no test files found"))
	}

	// TODO/FIXME density
	todoCount := countTODOs(repoPath)
	todoStatus := "pass"
	if todoCount > 20 {
		todoStatus = "warn"
	}
	results = append(results, newCheck("TODO/FIXME density", todoStatus, "info",
		fmt.Sprintf("%d TODO/FIXME occurrences in source", todoCount)))

	return results
}

func newCheck(label, status, severity, detail string) CheckResult {
	return CheckResult{label, status, severity, detail}
}

func fileCheck(repoPath, filename, label, severity string) CheckResult {
	path := filepath.Join(repoPath, filename)
	if _, err := os.Stat(path); err == nil {
		return newCheck(label, "pass", severity, filename+" found")
	}
	return newCheck(label, "warn", severity, filename+" not found")
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func countFiles(root string, match func(string) bool) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !d.IsDir() && match(d.Name()) {
			count++
		}
		return nil
	})
	return count
}

func countTODOs(root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext != ".go" && ext != ".rs" && ext != ".ts" && ext != ".py" && ext != ".js" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		count += strings.Count(content, "TODO") + strings.Count(content, "FIXME")
		return nil
	})
	return count
}

func formatChecks(checks []CheckResult) string {
	var sb strings.Builder
	for _, c := range checks {
		fmt.Fprintf(&sb, "[%s] %s (%s): %s\n", c.Status, c.Name, c.Severity, c.Detail)
	}
	return sb.String()
}
