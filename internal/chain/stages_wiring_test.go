// internal/chain/stages_wiring_test.go
package chain_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/chain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCouncilRunner(t *testing.T) {
	// Stub council runner that always succeeds.
	stub := chain.StageRunnerFunc(func(ctx context.Context, prior []chain.Result) chain.Result {
		return chain.Result{Stage: "council", Output: "health: 0.9", Payload: &chain.CouncilPayload{HealthScore: 0.9}}
	})
	result := stub.Run(context.Background(), nil)
	require.NoError(t, result.Err)
	p, ok := result.Payload.(*chain.CouncilPayload)
	assert.True(t, ok)
	assert.InDelta(t, 0.9, p.HealthScore, 0.001)
}

func TestWiredStageSetsCorrectStageName(t *testing.T) {
	// Verify BuildStageRunners returns runners keyed to canonical stage names.
	cfg := chain.StageWiringConfig{
		RepoPath:     t.TempDir(),
		AnthropicKey: "key",
		OpenAIKey:    "key",
	}
	runners := chain.BuildStageRunners(cfg)
	for _, name := range chain.CanonicalOrder() {
		_, ok := runners[name]
		assert.True(t, ok, "missing runner for stage %q", name)
	}
}
