package profile_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/ops/profile"
)

type stubRunner struct{ response string; err error }

func (s *stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, s.err
}

func TestRunReturnsResult(t *testing.T) {
	runner := &stubRunner{response: "## Hotspots\n`encoding/json.Marshal` — 42% CPU\n\n## Optimization Suggestions\nUse a custom marshaller."}
	result, err := profile.Run(context.Background(), profile.Config{
		Input:  "Showing top 10 nodes\n  flat  flat%   sum%        cum   cum%\n  420ms 42.00% 42.00%      420ms 42.00%  encoding/json.Marshal",
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
	_, err := profile.Run(context.Background(), profile.Config{
		Input:  "some profile",
		Runner: runner,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildPromptContainsInput(t *testing.T) {
	var captured string
	runner := profile.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		captured = prompt
		return "ok", nil
	})
	_, _ = profile.Run(context.Background(), profile.Config{
		Input:  "BenchmarkFoo-8   1000000   1200 ns/op   512 B/op",
		Runner: runner,
	})
	if !strings.Contains(captured, "BenchmarkFoo-8") {
		t.Errorf("prompt missing input content; got: %s", captured)
	}
	if !strings.Contains(captured, "Hotspots") {
		t.Errorf("prompt missing Hotspots instruction; got: %s", captured)
	}
}

func TestInputCapAt30KB(t *testing.T) {
	var captured string
	runner := profile.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		captured = prompt
		return "ok", nil
	})
	bigInput := strings.Repeat("flat 100ms 10.00% funcName\n", 2000) // ~60KB
	_, _ = profile.Run(context.Background(), profile.Config{
		Input:  bigInput,
		Runner: runner,
	})
	if !strings.Contains(captured, "[truncated]") {
		t.Errorf("expected truncation marker for large input")
	}
}
