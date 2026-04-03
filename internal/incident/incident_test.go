package incident_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/incident"
)

type stubRunner struct{ response string; err error }

func (s *stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, s.err
}

func TestRunReturnsResult(t *testing.T) {
	runner := &stubRunner{response: "## Timeline\n14:00 - Alert fired\n## Root Cause\nDB overload\n## Impact\n500 errors\n## Mitigations Applied\nRestarted service\n## Follow-up Actions\n1. Add circuit breaker"}
	result, err := incident.Run(context.Background(), incident.Config{
		Description: "Database became unresponsive causing 500 errors for 30 minutes.",
		Runner:      runner,
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
	_, err := incident.Run(context.Background(), incident.Config{
		Description: "Something broke.",
		Runner:      runner,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildPromptContainsDescriptionAndLogs(t *testing.T) {
	var captured string
	runner := incident.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		captured = prompt
		return "ok", nil
	})
	_, _ = incident.Run(context.Background(), incident.Config{
		Description: "Redis cluster failed over unexpectedly.",
		Logs:        "WARN redis: connection lost\nERROR failover triggered",
		Runner:      runner,
	})
	if !strings.Contains(captured, "Redis cluster failed over") {
		t.Errorf("prompt missing description; got: %s", captured)
	}
	if !strings.Contains(captured, "failover triggered") {
		t.Errorf("prompt missing log content; got: %s", captured)
	}
	if !strings.Contains(captured, "## Root Cause") {
		t.Errorf("prompt missing section header; got: %s", captured)
	}
}

func TestBuildPromptWithoutLogs(t *testing.T) {
	var captured string
	runner := incident.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		captured = prompt
		return "ok", nil
	})
	_, _ = incident.Run(context.Background(), incident.Config{
		Description: "Service outage.",
		Runner:      runner,
	})
	if strings.Contains(captured, "Supporting Logs") {
		t.Errorf("prompt should not include logs section when logs are empty; got: %s", captured)
	}
}
