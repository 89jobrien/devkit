// internal/chain/result_test.go
package chain_test

import (
	"errors"
	"testing"

	"github.com/89jobrien/devkit/internal/chain"
	"github.com/stretchr/testify/assert"
)

func TestResultIsSkipped(t *testing.T) {
	r := chain.Result{}
	assert.True(t, r.IsSkipped())
}

func TestResultNotSkippedWhenOutput(t *testing.T) {
	r := chain.Result{Stage: "council", Output: "some output"}
	assert.False(t, r.IsSkipped())
}

func TestResultNotSkippedWhenError(t *testing.T) {
	r := chain.Result{Stage: "council", Err: errors.New("failed")}
	assert.False(t, r.IsSkipped())
}

func TestCouncilPayload(t *testing.T) {
	p := &chain.CouncilPayload{HealthScore: 0.87, RoleOutputs: map[string]string{"strict-critic": "text"}}
	r := chain.Result{Stage: "council", Output: "output", Payload: p}
	got, ok := r.Payload.(*chain.CouncilPayload)
	assert.True(t, ok)
	assert.InDelta(t, 0.87, got.HealthScore, 0.001)
}
