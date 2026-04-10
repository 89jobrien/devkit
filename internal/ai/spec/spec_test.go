package spec_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

func TestLatestSpecFile(t *testing.T) {
	dir := t.TempDir()

	// Write three files with staggered mtimes.
	files := []string{"a.md", "b.md", "c.md"}
	for i, name := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("# "+name), 0o644))
		mtime := time.Now().Add(time.Duration(i) * time.Second)
		require.NoError(t, os.Chtimes(path, mtime, mtime))
	}

	got, err := spec.LatestSpecFile(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "c.md"), got)
}

func TestLatestSpecFileEmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := spec.LatestSpecFile(dir)
	assert.Error(t, err)
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

func TestParseHealthScore(t *testing.T) {
	cases := []struct {
		input    string
		expected float64
	}{
		{"**Health Score:** 0.72", 0.72},
		{"**Health Score:** 1.0", 1.0},
		{"**Health Score:** 0.0", 0.0},
		{"no score here", 0.5},           // default
		{"**health score:** 0.88", 0.88}, // case-insensitive
	}
	for _, c := range cases {
		got := spec.ParseHealthScore(c.input)
		assert.InDelta(t, c.expected, got, 0.001, "input: %q", c.input)
	}
}

func TestMetaScore(t *testing.T) {
	outputs := map[string]string{
		"completeness": "**Health Score:** 0.6",
		"ambiguity":    "**Health Score:** 0.9",
		"scope":        "**Health Score:** 0.8",
		"critic":       "**Health Score:** 0.7",
		"creative":     "**Health Score:** 0.85",
		"generalist":   "**Health Score:** 0.75",
	}
	score := spec.MetaScore(outputs)
	// average: (0.6+0.9+0.8+0.7+0.85+0.75)/6 = 0.7667
	assert.InDelta(t, 0.7667, score, 0.01)
}

func TestSynthesize(t *testing.T) {
	capture := &captureRunner{}
	outputs := map[string]string{
		"completeness": "**Health Score:** 0.6\n**Summary**\nMissing sections.",
		"critic":       "**Health Score:** 0.5\n**Summary**\nCritical gaps.",
	}
	result, err := spec.Synthesize(context.Background(), outputs, "docs/specs/test.md", capture)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	require.Len(t, capture.prompts, 1)
	p := capture.prompts[0]
	assert.Contains(t, p, "Completeness Checker")
	assert.Contains(t, p, "Strict Critic")
	assert.Contains(t, p, "**Health Scores**")
	assert.Contains(t, p, "**Spec Health**")
}
