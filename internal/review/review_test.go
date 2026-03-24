// internal/review/review_test.go
package review_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRunner struct{ response string }

func (s stubRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return s.response, nil
}

func TestReviewReturnsOutput(t *testing.T) {
	r := stubRunner{response: "No issues found."}
	result, err := review.Run(context.Background(), review.Config{
		Base:   "main",
		Diff:   "diff --git a/main.go b/main.go",
		Runner: r,
	})
	require.NoError(t, err)
	assert.Equal(t, "No issues found.", result)
}

func TestReviewUsesCustomFocus(t *testing.T) {
	var capturedPrompt string
	r := review.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		capturedPrompt = prompt
		return "ok", nil
	})
	_, _ = review.Run(context.Background(), review.Config{
		Base: "main", Diff: "diff", Focus: "- Rust unsafe", Runner: r,
	})
	assert.Contains(t, capturedPrompt, "Rust unsafe")
}
