package changelog_test

import (
	"context"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/changelog"
)

type stubRunner struct{ response string }

func (s *stubRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	return s.response, nil
}

func TestRunConventional(t *testing.T) {
	runner := &stubRunner{response: "## [v1.1.0] — 2026-04-02\n### Features\n- add changelog"}
	result, err := changelog.Run(context.Background(), changelog.Config{
		Log:    "feat: add changelog\nfix: typo",
		Format: "conventional",
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestRunProse(t *testing.T) {
	runner := &stubRunner{response: "## Release: Changelog agent\n### Summary\nAdds changelog generation."}
	result, err := changelog.Run(context.Background(), changelog.Config{
		Log:    "feat: add changelog",
		Format: "prose",
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestBuildPromptIncludesLog(t *testing.T) {
	called := false
	var capturedPrompt string
	runner := changelog.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		called = true
		capturedPrompt = prompt
		return "output", nil
	})
	_, _ = changelog.Run(context.Background(), changelog.Config{
		Log:    "feat: my-feature",
		Format: "conventional",
		Runner: runner,
	})
	if !called {
		t.Fatal("runner was not called")
	}
	if !strings.Contains(capturedPrompt, "feat: my-feature") {
		t.Errorf("prompt missing git log; got: %s", capturedPrompt)
	}
}
