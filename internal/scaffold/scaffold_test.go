package scaffold_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/scaffold"
)

type stubRunner struct{ response string; err error }

func (s *stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, s.err
}

func TestRunReturnsResult(t *testing.T) {
	runner := &stubRunner{response: "package mypackage\n\ntype Runner interface { Run(ctx context.Context, prompt string, tools []string) (string, error) }"}
	result, err := scaffold.Run(context.Background(), scaffold.Config{
		Name:    "mypackage",
		Purpose: "Analyze code complexity.",
		Runner:  runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestRunPropagatesError(t *testing.T) {
	runner := &stubRunner{err: errors.New("llm unavailable")}
	_, err := scaffold.Run(context.Background(), scaffold.Config{
		Name:    "mypkg",
		Purpose: "Does something.",
		Runner:  runner,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildPromptContainsNameAndPurpose(t *testing.T) {
	var captured string
	runner := scaffold.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		captured = prompt
		return "ok", nil
	})
	_, _ = scaffold.Run(context.Background(), scaffold.Config{
		Name:        "analyzer",
		Purpose:     "Detect duplicate code blocks.",
		RepoContext: "hexagonal Go project",
		Runner:      runner,
	})
	if !strings.Contains(captured, "analyzer") {
		t.Errorf("prompt missing package name; got: %s", captured)
	}
	if !strings.Contains(captured, "Detect duplicate code blocks") {
		t.Errorf("prompt missing purpose; got: %s", captured)
	}
	if !strings.Contains(captured, "Runner") {
		t.Errorf("prompt missing Runner interface instruction; got: %s", captured)
	}
}
