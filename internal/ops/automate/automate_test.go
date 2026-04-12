package automate_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/ops/automate"
)

type stubRunner struct{}

func (s *stubRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	return "stub output", nil
}

func TestRunRequiresRunner(t *testing.T) {
	_, err := automate.Run(context.Background(), automate.Config{
		Tasks:    []string{"changelog"},
		RepoPath: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error when runner is nil")
	}
}

func TestRunChangelogTask(t *testing.T) {
	result, err := automate.Run(context.Background(), automate.Config{
		Tasks:    []string{"changelog"},
		RepoPath: t.TempDir(),
		Runner:   &stubRunner{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "## Changelog") {
		t.Errorf("expected Changelog section, got: %s", result)
	}
}

func TestRunUnknownTask(t *testing.T) {
	result, err := automate.Run(context.Background(), automate.Config{
		Tasks:    []string{"nonexistent"},
		RepoPath: t.TempDir(),
		Runner:   &stubRunner{},
	})
	if err == nil {
		t.Fatal("expected error for unknown task, got nil")
	}
	if !strings.Contains(result, "unknown task") {
		t.Errorf("expected unknown task message in output, got: %s", result)
	}
}

func TestRunTaskHandlerError(t *testing.T) {
	// A runner that always returns an error simulates a registered task failing
	// (e.g. the LLM call itself fails). Run must surface a non-nil error so
	// callers are not falsely signalled success.
	runner := automate.RunnerFunc(func(_ context.Context, _ string, _ []string) (string, error) {
		return "", fmt.Errorf("llm unavailable")
	})
	result, err := automate.Run(context.Background(), automate.Config{
		Tasks:    []string{"changelog"},
		RepoPath: t.TempDir(),
		Runner:   runner,
	})
	if err == nil {
		t.Fatal("expected error when task handler fails, got nil")
	}
	if !strings.Contains(err.Error(), "changelog") {
		t.Errorf("error should name the failing task, got: %v", err)
	}
	if !strings.Contains(result, "## Changelog") {
		t.Errorf("output should include the task heading, got: %s", result)
	}
}

func TestRunnerFuncAdapter(t *testing.T) {
	called := false
	runner := automate.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		called = true
		return "ok", nil
	})
	_, _ = automate.Run(context.Background(), automate.Config{
		Tasks:    []string{"changelog"},
		RepoPath: t.TempDir(),
		Runner:   runner,
	})
	if !called {
		t.Fatal("RunnerFunc was not called")
	}
}
