// internal/council/council_test.go
package council_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/89jobrien/devkit/internal/ai/council"
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

func TestRunCoreRolesConcurrently(t *testing.T) {
	runner := stubRunner{response: "**Health Score:** 0.8\n**Summary**\nLooks good."}
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
	runner := stubRunner{response: "**Health Score:** 0.7\n**Summary**\nOK."}
	result, err := council.Run(context.Background(), council.Config{
		Base: "main", Mode: "extensive", Diff: "diff", Commits: "abc", Runner: runner,
	})
	require.NoError(t, err)
	assert.Len(t, result.RoleOutputs, 5)
}

func TestRunPerRoleOverride(t *testing.T) {
	defaultRunner := stubRunner{response: "**Health Score:** 0.5\n**Summary**\ndefault"}
	capture := &captureRunner{}
	result, err := council.Run(context.Background(), council.Config{
		Base:   "main",
		Mode:   "core",
		Diff:   "diff",
		Runner: defaultRunner,
		Runners: map[string]council.Runner{
			"strict-critic": capture,
		},
	})
	require.NoError(t, err)
	assert.Len(t, capture.prompts, 1, "strict-critic should use override runner")
	assert.NotEmpty(t, result.RoleOutputs["strict-critic"])
}

func TestRunNilRunnerReturnsError(t *testing.T) {
	_, err := council.Run(context.Background(), council.Config{
		Base: "main", Mode: "core", Diff: "diff",
		Runner: nil,
	})
	assert.Error(t, err)
}

func TestRunRoleErrorPropagates(t *testing.T) {
	_, err := council.Run(context.Background(), council.Config{
		Base: "main", Mode: "core", Diff: "diff",
		Runner: errRunner{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider down")
}

func TestRolePromptContainsTemplate(t *testing.T) {
	capture := &captureRunner{}
	_, err := council.Run(context.Background(), council.Config{
		Base:   "main",
		Mode:   "core",
		Diff:   "diff --git a/foo.go +added",
		Runner: capture,
	})
	require.NoError(t, err)
	require.Len(t, capture.prompts, 3)
	for _, prompt := range capture.prompts {
		assert.Contains(t, prompt, "**Health Score:**", "each role prompt must include Health Score template marker")
		assert.Contains(t, prompt, "**Summary**", "each role prompt must include Summary template section")
		assert.Contains(t, prompt, "**Recommendations**", "each role prompt must include Recommendations section")
	}
}

func TestRolePromptContainsDiff(t *testing.T) {
	capture := &captureRunner{}
	_, err := council.Run(context.Background(), council.Config{
		Base: "main", Mode: "core",
		Diff:    "diff --git a/bar.go",
		Commits: "deadbeef fix bar",
		Runner:  capture,
	})
	require.NoError(t, err)
	for _, prompt := range capture.prompts {
		assert.Contains(t, prompt, "diff --git a/bar.go")
		assert.Contains(t, prompt, "deadbeef fix bar")
	}
}

func TestStrictCriticPromptContainsRisks(t *testing.T) {
	capture := &captureRunner{}
	_, err := council.Run(context.Background(), council.Config{
		Base: "main", Mode: "core", Diff: "diff", Runner: capture,
	})
	require.NoError(t, err)
	var criticPrompt string
	for _, p := range capture.prompts {
		if strings.Contains(p, "STRICT CRITIC") {
			criticPrompt = p
			break
		}
	}
	require.NotEmpty(t, criticPrompt, "expected strict-critic prompt")
	assert.Contains(t, criticPrompt, "**Risks Identified**")
	assert.Contains(t, criticPrompt, "**Key Observations**")
}

func TestCreativeExplorerPromptContainsInnovation(t *testing.T) {
	capture := &captureRunner{}
	_, err := council.Run(context.Background(), council.Config{
		Base: "main", Mode: "core", Diff: "diff", Runner: capture,
	})
	require.NoError(t, err)
	var explorerPrompt string
	for _, p := range capture.prompts {
		if strings.Contains(p, "CREATIVE EXPLORER") {
			explorerPrompt = p
			break
		}
	}
	require.NotEmpty(t, explorerPrompt)
	assert.Contains(t, explorerPrompt, "**Innovation Opportunities**")
	assert.Contains(t, explorerPrompt, "**Architectural Potential**")
}

func TestSecurityReviewerPromptCap(t *testing.T) {
	capture := &captureRunner{}
	_, err := council.Run(context.Background(), council.Config{
		Base: "main", Mode: "extensive", Diff: "diff", Runner: capture,
	})
	require.NoError(t, err)
	var secPrompt string
	for _, p := range capture.prompts {
		if strings.Contains(p, "SECURITY REVIEWER") {
			secPrompt = p
			break
		}
	}
	require.NotEmpty(t, secPrompt)
	assert.Contains(t, secPrompt, "0.4", "security reviewer prompt must mention critical finding cap")
	assert.Contains(t, secPrompt, "**Findings**")
}

func TestMetaScore(t *testing.T) {
	outputs := map[string]string{
		"strict-critic":     "**Health Score:** 0.6",
		"creative-explorer": "**Health Score:** 0.9",
		"general-analyst":   "**Health Score:** 0.8",
	}
	score := council.MetaScore(outputs)
	// simple average: (0.6 + 0.9 + 0.8) / 3 ≈ 0.767
	assert.InDelta(t, 0.767, score, 0.01)
}

func TestParseHealthScore(t *testing.T) {
	cases := []struct {
		input    string
		expected float64
	}{
		{"**Health Score:** 0.72", 0.72},
		{"**Health Score:** 1.0", 1.0},
		{"**Health Score:** 0.0", 0.0},
		{"no score here", 0.5}, // default
		{"**health score:** 0.88", 0.88}, // case-insensitive
	}
	for _, c := range cases {
		got := council.ParseHealthScore(c.input)
		assert.InDelta(t, c.expected, got, 0.001, "input: %q", c.input)
	}
}

func TestSynthesize(t *testing.T) {
	capture := &captureRunner{}
	outputs := map[string]string{
		"strict-critic":     "**Health Score:** 0.6\n**Summary**\nStrict view.",
		"creative-explorer": "**Health Score:** 0.9\n**Summary**\nOptimistic view.",
	}
	result, err := council.Synthesize(context.Background(), outputs, council.Config{
		Base: "main", Diff: "diff --git a/x.go", Commits: "abc fix",
	}, capture)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	require.Len(t, capture.prompts, 1)
	synthPrompt := capture.prompts[0]
	assert.Contains(t, synthPrompt, "Strict Critic")
	assert.Contains(t, synthPrompt, "Creative Explorer")
	assert.Contains(t, synthPrompt, "**Health Scores**")
	assert.Contains(t, synthPrompt, "**Branch Health**")
}

func TestPersonasExported(t *testing.T) {
	p, ok := council.Personas["strict-critic"]
	if !ok {
		t.Fatal("Personas missing strict-critic")
	}
	if p == "" {
		t.Fatal("strict-critic persona is empty")
	}
	for _, key := range []string{"strict-critic", "security-reviewer", "performance-analyst"} {
		if _, ok := council.Personas[key]; !ok {
			t.Fatalf("Personas missing %s", key)
		}
	}
}
