// internal/chain/pipeline.go
package chain

import "context"

// RunPipeline executes the pipeline:
//
//	results[0]         = preflight Result (Stage="preflight")
//	results[1..N]      = one slot per StageSlot (zero Result if not Selected)
//	results[N+1]       = synthesis Result (always runs)
//
// Errors from individual stages are captured in their Result.Err — the pipeline
// never aborts early. Synthesis always receives the full results slice.
func RunPipeline(ctx context.Context, slots []StageSlot, synthesis StageRunner) ([]Result, error) {
	results := make([]Result, 1+len(slots)+1)

	// Index 0: preflight (recorded as a no-op pass here; cmd layer runs real preflight before calling RunPipeline).
	results[0] = Result{Stage: "preflight", Output: "ok"}

	// Indices 1..N: stage slots.
	for i, slot := range slots {
		if !slot.Selected || slot.Runner == nil {
			// Leave as zero Result — IsSkipped() returns true.
			continue
		}
		prior := results[:i+1] // pass all results so far (read-only slice)
		results[i+1] = slot.Runner.Run(ctx, prior)
	}

	// Last index: synthesis always runs with all prior results.
	results[len(results)-1] = synthesis.Run(ctx, results[:len(results)-1])

	return results, nil
}
