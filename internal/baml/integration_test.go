//go:build integration

package baml_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/89jobrien/devkit/internal/baml"
)

// requireAPIKey skips the test if ANTHROPIC_API_KEY is not set.
func requireAPIKey(t *testing.T) {
	t.Helper()
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
}

// TestAdapterIntegrationStrictCritic runs one real BAML call.
// Requires: ANTHROPIC_API_KEY env var.
// Run with: go test ./internal/baml/... -tags integration
func TestAdapterIntegrationStrictCritic(t *testing.T) {
	requireAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var buf bytes.Buffer
	a := baml.New("strict-critic", &buf)

	result, err := a.Run(ctx, "Review this: simple hello world program with no tests.", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Validate required rubric sections are present.
	for _, section := range []string{"Health Score", "Summary", "Recommendations"} {
		if !strings.Contains(result, section) {
			t.Errorf("expected %q in result, got:\n%s", section, result)
		}
	}

	// Note: buf (the out io.Writer) is currently unused by the adapter —
	// streaming token forwarding is reserved for a future enhancement.
	// The final result is returned directly from the BAML drain functions.

	if testing.Verbose() {
		t.Logf("final result:\n%s", result)
	}
}
