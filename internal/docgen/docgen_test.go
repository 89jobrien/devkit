package docgen_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/docgen"
)

type stubRunner struct{ response string; err error }

func (s *stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, s.err
}

func TestRunReturnsResult(t *testing.T) {
	runner := &stubRunner{response: "// Package foo provides utilities.\n\n// Bar does bar things.\nfunc Bar() {}"}
	result, err := docgen.Run(context.Background(), docgen.Config{
		File:   "package foo\n\nfunc Bar() {}",
		Path:   "foo.go",
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
	_, err := docgen.Run(context.Background(), docgen.Config{
		File:   "package foo",
		Path:   "foo.go",
		Runner: runner,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildPromptContainsFileAndPath(t *testing.T) {
	var captured string
	runner := docgen.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		captured = prompt
		return "ok", nil
	})
	_, _ = docgen.Run(context.Background(), docgen.Config{
		File:   "package util\n\nfunc Dedupe(s []string) []string { return s }",
		Path:   "util/dedupe.go",
		Runner: runner,
	})
	if !strings.Contains(captured, "util/dedupe.go") {
		t.Errorf("prompt missing path; got: %s", captured)
	}
	if !strings.Contains(captured, "func Dedupe") {
		t.Errorf("prompt missing file content; got: %s", captured)
	}
	if !strings.Contains(captured, "GoDoc") {
		t.Errorf("prompt missing GoDoc instruction; got: %s", captured)
	}
}
