package health_test

import (
	"context"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/ops/health"
)

type stubRunner struct{ response string }

func (s *stubRunner) Run(_ context.Context, repoContext, checkResults string) (string, error) {
	return s.response + " | ctx=" + repoContext, nil
}

func TestRunRequiresRunner(t *testing.T) {
	_, err := health.Run(context.Background(), health.Config{RepoPath: t.TempDir()})
	if err == nil {
		t.Fatal("expected error when runner is nil")
	}
}

func TestRunCallsRunner(t *testing.T) {
	dir := t.TempDir()
	runner := &stubRunner{response: "score:90"}
	result, err := health.Run(context.Background(), health.Config{
		RepoPath: dir,
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "score:90") {
		t.Errorf("expected runner response in result, got: %s", result)
	}
}

func TestRunnerFuncAdapter(t *testing.T) {
	called := false
	runner := health.RunnerFunc(func(_ context.Context, repoCtx, checks string) (string, error) {
		called = true
		return "ok", nil
	})
	_, _ = health.Run(context.Background(), health.Config{
		RepoPath: t.TempDir(),
		Runner:   runner,
	})
	if !called {
		t.Fatal("RunnerFunc was not called")
	}
}
