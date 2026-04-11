// internal/chain/stage_test.go
package chain_test

import (
	"testing"

	"github.com/89jobrien/devkit/internal/chain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixedOrderIsStable(t *testing.T) {
	// The canonical order must never change between calls.
	a := chain.CanonicalOrder()
	b := chain.CanonicalOrder()
	assert.Equal(t, a, b)
}

func TestCanonicalOrderContainsKnownStages(t *testing.T) {
	order := chain.CanonicalOrder()
	for _, name := range []string{"council", "ci-triage", "log-pattern", "diagnose", "ticket", "pr", "meta"} {
		assert.Contains(t, order, name, "canonical order missing stage %q", name)
	}
}

func TestSelectStages(t *testing.T) {
	slots, err := chain.SelectStages([]string{"council", "ticket"})
	require.NoError(t, err)
	// council is index 0, ticket is index 4 in canonical order
	assert.Equal(t, "council", slots[0].Name)
	assert.True(t, slots[0].Selected)
	assert.False(t, slots[1].Selected) // ci-triage skipped
	assert.Equal(t, "ticket", slots[4].Name)
	assert.True(t, slots[4].Selected)
}

func TestSelectStages_UnknownName(t *testing.T) {
	_, err := chain.SelectStages([]string{"council", "nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestSelectStages_Empty(t *testing.T) {
	_, err := chain.SelectStages([]string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one stage")
}
