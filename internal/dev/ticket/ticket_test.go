package ticket_test

import (
	"context"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/dev/ticket"
)

type stubRunner struct{ response string }

func (s *stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, nil
}

func TestPromptMode(t *testing.T) {
	runner := &stubRunner{response: "## Title\nAdd dark mode\n## Description\nUsers want dark mode."}
	result, err := ticket.Run(context.Background(), ticket.Config{
		Prompt: "add dark mode support",
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestCodeContextMode(t *testing.T) {
	runner := &stubRunner{response: "## Title\nFix TODO in router\n## Description\nFound TODO."}
	result, err := ticket.Run(context.Background(), ticket.Config{
		Prompt: "found TODO: handle nil pointer in Route()",
		Path:   "internal/router/router.go",
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestEmptyPromptReturnsError(t *testing.T) {
	_, err := ticket.Run(context.Background(), ticket.Config{
		Runner: &stubRunner{},
	})
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestPromptModePromptIncludesInput(t *testing.T) {
	var capturedPrompt string
	runner := ticket.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		capturedPrompt = prompt
		return "output", nil
	})
	_, _ = ticket.Run(context.Background(), ticket.Config{
		Prompt: "support webhook retries",
		Runner: runner,
	})
	if !strings.Contains(capturedPrompt, "support webhook retries") {
		t.Errorf("prompt missing user input; got: %s", capturedPrompt)
	}
}
