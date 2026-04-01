package baml_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/baml"
)

func TestAdapterRunReturnsMarkdown(t *testing.T) {
	var buf bytes.Buffer
	a := baml.NewWithStub("strict-critic", &buf, func(_ context.Context, _, _ string) (string, error) {
		return "**Health Score:** 0.80\n\n**Summary:**\nlooks good\n\n**Recommendations:**\n- add tests\n\n**Risks:**\n- none\n", nil
	})

	result, err := a.Run(context.Background(), "some prompt", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Health Score") {
		t.Errorf("expected markdown with Health Score, got: %s", result)
	}
	if !strings.Contains(result, "looks good") {
		t.Errorf("expected summary in result, got: %s", result)
	}
}

func TestAdapterRunUnknownRoleFallsBack(t *testing.T) {
	var buf bytes.Buffer
	a := baml.NewWithStub("unknown-role", &buf, func(_ context.Context, _, _ string) (string, error) {
		return "**Health Score:** 0.50\n\n**Summary:**\nok\n", nil
	})

	_, err := a.Run(context.Background(), "prompt", nil)
	if err != nil {
		t.Fatalf("unexpected error for unknown role: %v", err)
	}
}

func TestAdapterRunPropagatesError(t *testing.T) {
	var buf bytes.Buffer
	a := baml.NewWithStub("strict-critic", &buf, func(_ context.Context, _, _ string) (string, error) {
		return "", context.DeadlineExceeded
	})

	_, err := a.Run(context.Background(), "prompt", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
