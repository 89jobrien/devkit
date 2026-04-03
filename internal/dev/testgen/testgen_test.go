package testgen_test

import (
	"context"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/dev/testgen"
)

type stubRunner struct{ response string }

func (s *stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, nil
}

func TestFileMode(t *testing.T) {
	runner := &stubRunner{response: "package foo_test\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {}"}
	result, err := testgen.Run(context.Background(), testgen.Config{
		File:   "package foo\n\nfunc Foo() string { return \"foo\" }",
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

func TestDiffMode(t *testing.T) {
	runner := &stubRunner{response: "package bar_test\n\nfunc TestBar(t *testing.T) {}"}
	result, err := testgen.Run(context.Background(), testgen.Config{
		Diff:   "diff --git a/bar.go\n+func Bar() {}",
		Log:    "feat: add Bar",
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
	_, err := testgen.Run(context.Background(), testgen.Config{
		File:   "package foo",
		Diff:   "diff --git",
		Runner: &stubRunner{},
	})
	if err == nil {
		t.Fatal("expected error when both File and Diff are set")
	}
}

func TestNeitherModeSetReturnsError(t *testing.T) {
	_, err := testgen.Run(context.Background(), testgen.Config{
		Runner: &stubRunner{},
	})
	if err == nil {
		t.Fatal("expected error when neither File nor Diff is set")
	}
}

func TestFileModePromptIncludesPath(t *testing.T) {
	var capturedPrompt string
	runner := testgen.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		capturedPrompt = prompt
		return "output", nil
	})
	_, _ = testgen.Run(context.Background(), testgen.Config{
		File:   "package foo\n\nfunc Baz() {}",
		Path:   "internal/foo/foo.go",
		Runner: runner,
	})
	if !strings.Contains(capturedPrompt, "internal/foo/foo.go") {
		t.Errorf("prompt missing path; got: %s", capturedPrompt)
	}
}
