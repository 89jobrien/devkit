package automate_test

import (
	"context"
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "unknown task") {
		t.Errorf("expected unknown task message, got: %s", result)
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
