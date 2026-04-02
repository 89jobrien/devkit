package lint_test

import (
	"context"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/lint"
)

type stubRunner struct{ response string }

func (s *stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, nil
}

func TestRunReturnsResult(t *testing.T) {
	runner := &stubRunner{response: "## Lint Report: foo.go\n### Issues\nNo issues found.\n### Verdict\nClean."}
	result, err := lint.Run(context.Background(), lint.Config{
		File:   "package foo\n\nfunc Foo() {}",
		Path:   "foo.go",
		Role:   "strict-critic",
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestBuildPromptIncludesFileAndPersona(t *testing.T) {
	var capturedPrompt string
	runner := lint.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		capturedPrompt = prompt
		return "output", nil
	})
	_, _ = lint.Run(context.Background(), lint.Config{
		File:   "package foo\n\nfunc Secret() {}",
		Path:   "secret.go",
		Role:   "strict-critic",
		Runner: runner,
	})
	if !strings.Contains(capturedPrompt, "STRICT CRITIC") {
		t.Errorf("prompt missing persona; got: %s", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "secret.go") {
		t.Errorf("prompt missing file path; got: %s", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "func Secret()") {
		t.Errorf("prompt missing file content; got: %s", capturedPrompt)
	}
}

func TestUnknownRoleFallsBackToStrictCritic(t *testing.T) {
	var capturedPrompt string
	runner := lint.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		capturedPrompt = prompt
		return "output", nil
	})
	_, _ = lint.Run(context.Background(), lint.Config{
		File:   "package foo",
		Path:   "foo.go",
		Role:   "nonexistent-role",
		Runner: runner,
	})
	if !strings.Contains(capturedPrompt, "STRICT CRITIC") {
		t.Errorf("expected fallback to strict-critic persona; got: %s", capturedPrompt)
	}
}
