// internal/chain/pipeline_test.go
package chain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/89jobrien/devkit/internal/chain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeStubRunner(name, output string, err error) chain.StageRunnerFunc {
	return func(ctx context.Context, prior []chain.Result) chain.Result {
		return chain.Result{Stage: name, Output: output, Err: err}
	}
}

func TestPipelineRunsInOrder(t *testing.T) {
	var order []string
	makeOrdered := func(name string) chain.StageRunnerFunc {
		return func(ctx context.Context, prior []chain.Result) chain.Result {
			order = append(order, name)
			return chain.Result{Stage: name, Output: name + " output"}
		}
	}
	slots := []chain.StageSlot{
		{Name: "council", Selected: true, Runner: makeOrdered("council")},
		{Name: "ci-triage", Selected: false},
		{Name: "ticket", Selected: true, Runner: makeOrdered("ticket")},
	}
	synthesis := makeStubRunner("synthesis", "synth output", nil)
	results, err := chain.RunPipeline(context.Background(), slots, chain.StageRunnerFunc(synthesis))
	require.NoError(t, err)
	assert.Equal(t, []string{"council", "ticket"}, order)
	// results[0]=preflight, results[1]=council, results[2]=skipped, results[3]=ticket, results[N]=synthesis
	assert.Equal(t, "council", results[1].Stage)
	assert.True(t, results[2].IsSkipped())
	assert.Equal(t, "ticket", results[3].Stage)
	assert.Equal(t, "synthesis", results[len(results)-1].Stage)
}

func TestPipelineSkippedSlotIsNilResult(t *testing.T) {
	slots := []chain.StageSlot{
		{Name: "council", Selected: false},
		{Name: "ci-triage", Selected: true, Runner: makeStubRunner("ci-triage", "out", nil)},
	}
	synthesis := makeStubRunner("synthesis", "synth", nil)
	results, _ := chain.RunPipeline(context.Background(), slots, chain.StageRunnerFunc(synthesis))
	assert.True(t, results[1].IsSkipped(), "council slot should be skipped (zero Result)")
	assert.Equal(t, "ci-triage", results[2].Stage)
}

func TestPipelineContinuesAfterStageError(t *testing.T) {
	slots := []chain.StageSlot{
		{Name: "council", Selected: true, Runner: makeStubRunner("council", "", errors.New("council failed"))},
		{Name: "ci-triage", Selected: true, Runner: makeStubRunner("ci-triage", "ci output", nil)},
	}
	synthesis := makeStubRunner("synthesis", "synth", nil)
	results, err := chain.RunPipeline(context.Background(), slots, chain.StageRunnerFunc(synthesis))
	// Pipeline does not abort on stage error — synthesis always runs.
	require.NoError(t, err)
	assert.NotNil(t, results[1].Err)
	assert.Equal(t, "ci-triage", results[2].Stage)
	assert.Equal(t, "synthesis", results[len(results)-1].Stage)
}

func TestPipelineResultsLengthIsSlotsPlusTwoFixed(t *testing.T) {
	// Always: preflight(0) + len(slots) + synthesis(last)
	slots := []chain.StageSlot{
		{Name: "council", Selected: true, Runner: makeStubRunner("council", "out", nil)},
		{Name: "ci-triage", Selected: false},
	}
	synthesis := makeStubRunner("synthesis", "synth", nil)
	results, _ := chain.RunPipeline(context.Background(), slots, chain.StageRunnerFunc(synthesis))
	assert.Len(t, results, 1+len(slots)+1) // preflight + slots + synthesis
}
