package spec_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/89jobrien/devkit/internal/ai/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRunner struct{ response string }

func (s stubRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	return s.response, nil
}

type captureRunner struct {
	mu      sync.Mutex
	prompts []string
}

func (c *captureRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	c.mu.Lock()
	c.prompts = append(c.prompts, prompt)
	c.mu.Unlock()
	return "**Health Score:** 0.75\n**Summary**\nok", nil
}

type errRunner struct{}

func (e errRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return "", errors.New("provider down")
}

func TestRunAllSixRoles(t *testing.T) {
	runner := stubRunner{response: "**Health Score:** 0.8\n**Summary**\nLooks good."}
	result, err := spec.Run(context.Background(), spec.Config{
		Content: "# My Spec\n\n## Problem\nSomething.\n\n## Design\nStuff.",
		Path:    "docs/superpowers/specs/test.md",
		Runner:  runner,
	})
	require.NoError(t, err)
	assert.Len(t, result.RoleOutputs, 6)
	for _, key := range []string{"completeness", "ambiguity", "scope", "critic", "creative", "generalist"} {
		assert.NotEmpty(t, result.RoleOutputs[key], "missing output for role %q", key)
	}
}

func TestRunNilRunnerReturnsError(t *testing.T) {
	_, err := spec.Run(context.Background(), spec.Config{
		Content: "# Spec",
		Runner:  nil,
	})
	assert.Error(t, err)
}

func TestRunRoleErrorPropagates(t *testing.T) {
	_, err := spec.Run(context.Background(), spec.Config{
		Content: "# Spec",
		Runner:  errRunner{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider down")
}

func TestRunPerRoleOverride(t *testing.T) {
	defaultRunner := stubRunner{response: "**Health Score:** 0.5\n**Summary**\ndefault"}
	capture := &captureRunner{}
	result, err := spec.Run(context.Background(), spec.Config{
		Content: "# Spec",
		Runner:  defaultRunner,
		Runners: map[string]spec.Runner{
			"critic": capture,
		},
	})
	require.NoError(t, err)
	assert.Len(t, capture.prompts, 1, "critic should use override runner")
	assert.NotEmpty(t, result.RoleOutputs["critic"])
}
