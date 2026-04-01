//go:build integration

package baml_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/baml"
)

// TestAdapterIntegrationStrictCritic runs one real BAML call.
// Requires: ANTHROPIC_API_KEY env var.
// Run with: go test ./internal/baml/... -tags integration
func TestAdapterIntegrationStrictCritic(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	var buf bytes.Buffer
	a := baml.New("strict-critic", &buf)

	result, err := a.Run(context.Background(), "Review this: simple hello world program with no tests.", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Health Score") {
		t.Errorf("expected Health Score in result, got: %s", result)
	}
	t.Logf("streamed tokens: %q", buf.String())
	t.Logf("final result: %s", result)
}
