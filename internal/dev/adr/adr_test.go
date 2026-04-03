package adr_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/dev/adr"
)

type stubRunner struct{ response string; err error }

func (s *stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, s.err
}

func TestRunReturnsResult(t *testing.T) {
	runner := &stubRunner{response: "## Status\nProposed\n## Context\nWe need X\n## Decision\nUse Y\n## Consequences\nFaster."}
	result, err := adr.Run(context.Background(), adr.Config{
		Title:   "Use PostgreSQL for persistent storage",
		Context: "We need a durable store for user data.",
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
	_, err := adr.Run(context.Background(), adr.Config{
		Title:  "Some decision",
		Runner: runner,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildPromptContainsTitleAndContext(t *testing.T) {
	var captured string
	runner := adr.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		captured = prompt
		return "ok", nil
	})
	_, _ = adr.Run(context.Background(), adr.Config{
		Title:   "Adopt hexagonal architecture",
		Context: "We want testable adapters.",
		Runner:  runner,
	})
	if !strings.Contains(captured, "Adopt hexagonal architecture") {
		t.Errorf("prompt missing title; got: %s", captured)
	}
	if !strings.Contains(captured, "testable adapters") {
		t.Errorf("prompt missing context; got: %s", captured)
	}
	if !strings.Contains(captured, "## Status") {
		t.Errorf("prompt missing section headers; got: %s", captured)
	}
}
