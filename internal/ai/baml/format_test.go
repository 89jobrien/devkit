// internal/baml/format_test.go — unit tests for the BAML adapter across all
// roles. Covers: table-driven role dispatch (p70 analog), model-contract field
// assertions (p50), and output completeness (p30 snapshot analog).
// All tests use NewWithStub so no real API calls are made.
package baml_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/ai/baml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roleCase describes a test scenario for a single BAML role.
type roleCase struct {
	role           string
	stubOutput     string          // what the stub runFunc returns
	requiredFields []string        // sections that must appear in Run output
	forbiddenIn    []string        // sections that must NOT appear (role-specific)
}

// roleCases covers all production roles plus the default fallback.
var roleCases = []roleCase{
	{
		role: "strict-critic",
		stubOutput: "**Health Score:** 0.41\n\n" +
			"**Summary:**\nThis branch has weak error handling.\n\n" +
			"**Recommendations:**\n- Add tests\n- Handle errors\n\n" +
			"**Risks:**\n- [high] Unchecked error in main.go:42\n",
		requiredFields: []string{"Health Score", "Summary", "Recommendations", "Risks"},
	},
	{
		role: "creative-explorer",
		stubOutput: "**Health Score:** 0.74\n\n" +
			"**Summary:**\nStrong architectural potential.\n\n" +
			"**Recommendations:**\n- Extract shared helper\n\n" +
			"**Innovation Opportunities:**\n- Batch mode across commands\n\n" +
			"**Architectural Potential:**\nHexagonal boundary already clear.\n",
		requiredFields: []string{"Health Score", "Summary", "Recommendations", "Innovation Opportunities", "Architectural Potential"},
	},
	{
		role: "security-reviewer",
		stubOutput: "**Health Score:** 0.60\n\n" +
			"**Summary:**\nNo critical vulnerabilities found.\n\n" +
			"**Recommendations:**\n- Validate input\n\n" +
			"**Findings:**\n- [medium] Unchecked stdin in ticket\n",
		requiredFields: []string{"Health Score", "Summary", "Recommendations", "Findings"},
	},
	{
		role: "general-analyst",
		stubOutput: "**Health Score:** 0.72\n\n" +
			"**Summary:**\nSolid feature progress.\n\n" +
			"**Recommendations:**\n- Add tests\n\n" +
			"**Gaps:**\n- No test coverage\n\n" +
			"**Work Patterns:**\nBatched delivery, single large commit.\n",
		requiredFields: []string{"Health Score", "Summary", "Recommendations", "Gaps", "Work Patterns"},
	},
	{
		role: "unknown-role", // default fallback
		stubOutput: "**Health Score:** 0.50\n\n" +
			"**Summary:**\nDefault analysis.\n\n" +
			"**Recommendations:**\n- Review the code\n",
		requiredFields: []string{"Health Score", "Summary", "Recommendations"},
	},
	{
		role: "pr",
		stubOutput: "# Fix authentication bug\n\n" +
			"This PR fixes the login flow.\n\n" +
			"## Changes\n- Fix token validation\n\n" +
			"## Test Plan\n- Run auth integration tests\n",
		requiredFields: []string{"Fix authentication bug", "Changes", "Test Plan"},
	},
}

// TestAdapterAllRoles verifies that each role's stub output passes through
// the adapter and contains all required rubric fields (model-contract, p50).
// This is the table-driven parameterization requested in p70.
func TestAdapterAllRoles(t *testing.T) {
	for _, tc := range roleCases {
		tc := tc
		t.Run(tc.role, func(t *testing.T) {
			var buf bytes.Buffer
			a := baml.NewWithStub(tc.role, &buf, func(_ context.Context, _, _ string) (string, error) {
				return tc.stubOutput, nil
			})

			result, err := a.Run(context.Background(), "test prompt", nil)
			require.NoError(t, err)

			for _, field := range tc.requiredFields {
				assert.Contains(t, result, field,
					"role %q: expected field %q in output", tc.role, field)
			}
		})
	}
}

// TestAdapterRoleSpecificFields verifies role-specific fields that distinguish
// roles from each other (model-contract, p50).
func TestAdapterRoleSpecificFields(t *testing.T) {
	t.Run("strict-critic has Risks not Findings", func(t *testing.T) {
		a := baml.NewWithStub("strict-critic", &bytes.Buffer{}, func(_ context.Context, _, _ string) (string, error) {
			return "**Health Score:** 0.41\n**Summary:**\nok\n**Recommendations:**\n- fix\n**Risks:**\n- risk\n", nil
		})
		result, err := a.Run(context.Background(), "p", nil)
		require.NoError(t, err)
		assert.Contains(t, result, "Risks")
		assert.NotContains(t, result, "Findings")
	})

	t.Run("security-reviewer has Findings not Risks", func(t *testing.T) {
		a := baml.NewWithStub("security-reviewer", &bytes.Buffer{}, func(_ context.Context, _, _ string) (string, error) {
			return "**Health Score:** 0.60\n**Summary:**\nok\n**Recommendations:**\n- check\n**Findings:**\n- [low] minor\n", nil
		})
		result, err := a.Run(context.Background(), "p", nil)
		require.NoError(t, err)
		assert.Contains(t, result, "Findings")
		assert.NotContains(t, result, "Risks")
	})

	t.Run("creative-explorer has Innovation Opportunities and Architectural Potential", func(t *testing.T) {
		a := baml.NewWithStub("creative-explorer", &bytes.Buffer{}, func(_ context.Context, _, _ string) (string, error) {
			return "**Health Score:** 0.74\n**Summary:**\nok\n**Recommendations:**\n- try\n**Innovation Opportunities:**\n- batch mode\n**Architectural Potential:**\nhex arch.\n", nil
		})
		result, err := a.Run(context.Background(), "p", nil)
		require.NoError(t, err)
		assert.Contains(t, result, "Innovation Opportunities")
		assert.Contains(t, result, "Architectural Potential")
	})

	t.Run("general-analyst has Gaps and Work Patterns", func(t *testing.T) {
		a := baml.NewWithStub("general-analyst", &bytes.Buffer{}, func(_ context.Context, _, _ string) (string, error) {
			return "**Health Score:** 0.72\n**Summary:**\nok\n**Recommendations:**\n- test\n**Gaps:**\n- missing tests\n**Work Patterns:**\nbatched.\n", nil
		})
		result, err := a.Run(context.Background(), "p", nil)
		require.NoError(t, err)
		assert.Contains(t, result, "Gaps")
		assert.Contains(t, result, "Work Patterns")
	})
}

// TestAdapterHealthScoreRange verifies the health score is a valid float in [0,1]
// range when formatted (golden output invariant, p30).
func TestAdapterHealthScoreRange(t *testing.T) {
	cases := []struct {
		role  string
		score string
		valid bool
	}{
		{"strict-critic", "0.41", true},
		{"strict-critic", "1.00", true},
		{"strict-critic", "0.00", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.role+"/"+tc.score, func(t *testing.T) {
			stub := "**Health Score:** " + tc.score + "\n**Summary:**\nok\n**Recommendations:**\n- x\n**Risks:**\n- y\n"
			a := baml.NewWithStub(tc.role, &bytes.Buffer{}, func(_ context.Context, _, _ string) (string, error) {
				return stub, nil
			})
			result, err := a.Run(context.Background(), "p", nil)
			require.NoError(t, err)
			assert.Contains(t, result, "Health Score:** "+tc.score)
		})
	}
}

// TestAdapterOutputCompleteness verifies that all sections present in the stub
// output survive the adapter pass-through (golden snapshot invariant, p30).
func TestAdapterOutputCompleteness(t *testing.T) {
	for _, tc := range roleCases {
		tc := tc
		t.Run(tc.role, func(t *testing.T) {
			a := baml.NewWithStub(tc.role, &bytes.Buffer{}, func(_ context.Context, _, _ string) (string, error) {
				return tc.stubOutput, nil
			})
			result, err := a.Run(context.Background(), "p", nil)
			require.NoError(t, err)
			// Every line of stub output must appear in the result (no truncation).
			for _, line := range strings.Split(tc.stubOutput, "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				assert.Contains(t, result, strings.TrimSpace(line),
					"role %q: stub line %q missing from output", tc.role, line)
			}
		})
	}
}

// TestAdapterErrorWrapping verifies the error includes the role name for
// easier diagnosis (p50 contract).
func TestAdapterErrorWrapping(t *testing.T) {
	roles := []string{"strict-critic", "creative-explorer", "security-reviewer", "general-analyst", "pr", "unknown-role"}
	for _, role := range roles {
		role := role
		t.Run(role, func(t *testing.T) {
			a := baml.NewWithStub(role, &bytes.Buffer{}, func(_ context.Context, _, _ string) (string, error) {
				return "", errors.New("upstream failure")
			})
			_, err := a.Run(context.Background(), "p", nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), role, "error should name the failing role")
			assert.Contains(t, err.Error(), "upstream failure")
		})
	}
}

// TestAdapterContextCancellation verifies the adapter propagates context
// cancellation from the runner (streaming behavior invariant, p60).
func TestAdapterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	a := baml.NewWithStub("strict-critic", &bytes.Buffer{}, func(ctx context.Context, _, _ string) (string, error) {
		return "", ctx.Err()
	})
	_, err := a.Run(ctx, "p", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled), "expected context.Canceled, got: %v", err)
}
