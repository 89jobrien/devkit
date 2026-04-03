package migrate_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/migrate"
)

type stubRunner struct{ response string; err error }

func (s *stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, s.err
}

func TestRunReturnsResult(t *testing.T) {
	runner := &stubRunner{response: "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-OldCall()\n+NewCall(ctx)"}
	result, err := migrate.Run(context.Background(), migrate.Config{
		Old:    "func OldCall()",
		New:    "func NewCall(ctx context.Context)",
		Code:   "package main\n\nfunc main() { OldCall() }",
		Path:   "main.go",
		Runner: runner,
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
	_, err := migrate.Run(context.Background(), migrate.Config{
		Old:    "old",
		New:    "new",
		Code:   "package main",
		Path:   "main.go",
		Runner: runner,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildPromptContainsAPIsAndCode(t *testing.T) {
	var captured string
	runner := migrate.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		captured = prompt
		return "ok", nil
	})
	_, _ = migrate.Run(context.Background(), migrate.Config{
		Old:    "func Foo(x int)",
		New:    "func Foo(ctx context.Context, x int)",
		Code:   "package bar\n\nfunc use() { Foo(42) }",
		Path:   "bar/use.go",
		Runner: runner,
	})
	if !strings.Contains(captured, "func Foo(x int)") {
		t.Errorf("prompt missing old API; got: %s", captured)
	}
	if !strings.Contains(captured, "func Foo(ctx context.Context") {
		t.Errorf("prompt missing new API; got: %s", captured)
	}
	if !strings.Contains(captured, "bar/use.go") {
		t.Errorf("prompt missing file path; got: %s", captured)
	}
	if !strings.Contains(captured, "unified diff") {
		t.Errorf("prompt missing diff instruction; got: %s", captured)
	}
}
