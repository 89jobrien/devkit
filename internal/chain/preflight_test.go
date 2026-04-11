// internal/chain/preflight_test.go
package chain_test

import (
	"testing"

	"github.com/89jobrien/devkit/internal/chain"
	"github.com/stretchr/testify/assert"
)

func TestPreflightPassesWithValidConfig(t *testing.T) {
	cfg := chain.PreflightConfig{
		Stages:       []string{"council"},
		AnthropicKey: "ant-key",
		OpenAIKey:    "oai-key",
		RepoPath:     t.TempDir(),
		LookupBinary: func(name string) bool { return true },
	}
	errs := chain.Preflight(cfg)
	assert.Empty(t, errs)
}

func TestPreflightReportsAllFailures(t *testing.T) {
	cfg := chain.PreflightConfig{
		Stages:       []string{"council", "badstage"},
		AnthropicKey: "",
		OpenAIKey:    "",
		RepoPath:     "/nonexistent/path/xyz",
		LookupBinary: func(name string) bool { return false },
	}
	errs := chain.Preflight(cfg)
	// Expect: unknown stage, missing keys, bad repo path — reported all at once.
	assert.Greater(t, len(errs), 1, "expected multiple errors reported at once")
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	combined := ""
	for _, m := range msgs {
		combined += m + "\n"
	}
	assert.Contains(t, combined, "badstage")
	assert.Contains(t, combined, "API key")
}

func TestPreflightRequiresGhForCITriage(t *testing.T) {
	ghPresent := false
	cfg := chain.PreflightConfig{
		Stages:       []string{"ci-triage"},
		AnthropicKey: "key",
		OpenAIKey:    "key",
		RepoPath:     t.TempDir(),
		LookupBinary: func(name string) bool {
			if name == "gh" {
				return ghPresent
			}
			return true
		},
	}
	errs := chain.Preflight(cfg)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "gh")
}
