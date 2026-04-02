package explain_test

import (
	"context"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/explain"
)

type stubRunner struct{ response string }

func (s *stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, nil
}

func TestFileMode(t *testing.T) {
	runner := &stubRunner{response: "## What It Does\nHandles routing."}
	result, err := explain.Run(context.Background(), explain.Config{
		File:   "package router\n\nfunc Route() {}",
		Path:   "router.go",
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestDiffMode(t *testing.T) {
	runner := &stubRunner{response: "## What Changed\n- added Route func"}
	result, err := explain.Run(context.Background(), explain.Config{
		Diff:   "diff --git a/router.go",
		Log:    "feat: add Route",
		Stat:   "router.go | 5 +++++",
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestBothModesSetReturnsError(t *testing.T) {
	runner := &stubRunner{}
	_, err := explain.Run(context.Background(), explain.Config{
		File:   "package foo",
		Diff:   "diff --git",
		Runner: runner,
	})
	if err == nil {
		t.Fatal("expected error when both File and Diff are set")
	}
}

func TestNeitherModeSetReturnsError(t *testing.T) {
	runner := &stubRunner{}
	_, err := explain.Run(context.Background(), explain.Config{
		Runner: runner,
	})
	if err == nil {
		t.Fatal("expected error when neither File nor Diff is set")
	}
}

func TestFileModePromptIncludesSymbol(t *testing.T) {
	var capturedPrompt string
	runner := explain.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		capturedPrompt = prompt
		return "output", nil
	})
	_, _ = explain.Run(context.Background(), explain.Config{
		File:   "package foo\n\nfunc MyFunc() {}",
		Path:   "foo.go",
		Symbol: "MyFunc",
		Runner: runner,
	})
	if !strings.Contains(capturedPrompt, "MyFunc") {
		t.Errorf("prompt missing symbol; got: %s", capturedPrompt)
	}
}
