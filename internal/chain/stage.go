// internal/chain/stage.go
package chain

import (
	"context"
	"fmt"
	"strings"
)

// StageRunner is the port for executing a single pipeline stage.
// Implementations call the underlying devkit command logic directly.
type StageRunner interface {
	Run(ctx context.Context, prior []Result) Result
}

// StageRunnerFunc adapts a function to StageRunner.
type StageRunnerFunc func(ctx context.Context, prior []Result) Result

func (f StageRunnerFunc) Run(ctx context.Context, prior []Result) Result {
	return f(ctx, prior)
}

// StageSlot represents one position in the fixed pipeline.
type StageSlot struct {
	Name     string
	Selected bool
	Runner   StageRunner // nil if not yet wired (set by cmd layer)
}

// canonicalOrder is the fixed execution order. Never reorder this slice.
var canonicalOrder = []string{
	"council",
	"ci-triage",
	"log-pattern",
	"diagnose",
	"ticket",
	"pr",
	"meta",
}

// CanonicalOrder returns the fixed stage execution order.
func CanonicalOrder() []string {
	out := make([]string, len(canonicalOrder))
	copy(out, canonicalOrder)
	return out
}

// SelectStages validates names and returns the full slot list in canonical order,
// with Selected=true for requested stages and Selected=false for skipped stages.
// Returns an error if any name is unknown or the list is empty.
func SelectStages(names []string) ([]StageSlot, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("chain: at least one stage required")
	}
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[strings.TrimSpace(n)] = true
	}
	// Validate all names against the canonical list.
	known := make(map[string]bool, len(canonicalOrder))
	for _, n := range canonicalOrder {
		known[n] = true
	}
	for n := range nameSet {
		if !known[n] {
			return nil, fmt.Errorf("chain: unknown stage %q (valid: %s)",
				n, strings.Join(canonicalOrder, ", "))
		}
	}
	slots := make([]StageSlot, len(canonicalOrder))
	for i, n := range canonicalOrder {
		slots[i] = StageSlot{Name: n, Selected: nameSet[n]}
	}
	return slots, nil
}
