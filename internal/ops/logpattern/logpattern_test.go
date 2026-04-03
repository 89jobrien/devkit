package logpattern_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/ops/logpattern"
)

type stubRunner struct{ response string; err error }

func (s *stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, s.err
}

func TestRunReturnsResult(t *testing.T) {
	runner := &stubRunner{response: "## Error Patterns\n\n### connection refused (42 occurrences)\nFirst: 2024-01-01T00:00:00Z\nLast: 2024-01-01T01:00:00Z"}
	result, err := logpattern.Run(context.Background(), logpattern.Config{
		Logs:   "2024-01-01T00:00:00Z ERROR connection refused\n2024-01-01T00:01:00Z ERROR connection refused",
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
	_, err := logpattern.Run(context.Background(), logpattern.Config{
		Logs:   "some logs",
		Runner: runner,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildPromptContainsLogs(t *testing.T) {
	var captured string
	runner := logpattern.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		captured = prompt
		return "ok", nil
	})
	_, _ = logpattern.Run(context.Background(), logpattern.Config{
		Logs:   "ERROR timeout waiting for lock\nERROR timeout waiting for lock",
		Runner: runner,
	})
	if !strings.Contains(captured, "timeout waiting for lock") {
		t.Errorf("prompt missing log content; got: %s", captured)
	}
	if !strings.Contains(captured, "frequency") {
		t.Errorf("prompt missing frequency instruction; got: %s", captured)
	}
}

func TestLogsCapAt50KB(t *testing.T) {
	var captured string
	runner := logpattern.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		captured = prompt
		return "ok", nil
	})
	bigLogs := strings.Repeat("ERROR something went wrong\n", 3000) // ~80KB
	_, _ = logpattern.Run(context.Background(), logpattern.Config{
		Logs:   bigLogs,
		Runner: runner,
	})
	if !strings.Contains(captured, "[truncated]") {
		t.Errorf("expected truncation marker in prompt for large input")
	}
}
