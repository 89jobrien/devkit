// internal/council/council_test.go
package council_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/council"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRunner struct{ response string }

func (s stubRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	return s.response, nil
}

func TestRunCoreRolesConcurrently(t *testing.T) {
	runner := stubRunner{response: "**Health Score**: 0.8\n## Summary\nLooks good."}
	result, err := council.Run(context.Background(), council.Config{
		Base:    "main",
		Mode:    "core",
		Diff:    "diff --git a/foo.go",
		Commits: "abc123 add foo",
		Runner:  runner,
	})
	require.NoError(t, err)
	assert.Len(t, result.RoleOutputs, 3)
	assert.NotEmpty(t, result.RoleOutputs["strict-critic"])
}

func TestRunExtensiveHasFiveRoles(t *testing.T) {
	runner := stubRunner{response: "**Health Score**: 0.7\n## Summary\nOK."}
	result, err := council.Run(context.Background(), council.Config{
		Base: "main", Mode: "extensive", Diff: "diff", Commits: "abc", Runner: runner,
	})
	require.NoError(t, err)
	assert.Len(t, result.RoleOutputs, 5)
}

func TestMetaScore(t *testing.T) {
	outputs := map[string]string{
		"strict-critic":     "**Health Score**: 0.6",
		"creative-explorer": "**Health Score**: 0.9",
		"general-analyst":   "**Health Score**: 0.8",
	}
	score := council.MetaScore(outputs)
	// strict-critic weight 1.5x: (0.6*1.5 + 0.9 + 0.8) / (1.5+1+1) = 2.6/3.5 ≈ 0.743
	assert.InDelta(t, 0.743, score, 0.01)
}
