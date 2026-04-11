// internal/repl/repl_test.go
package repl_test

import (
	"testing"

	"github.com/89jobrien/devkit/internal/chain"
	"github.com/89jobrien/devkit/internal/repl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCommand_Basic(t *testing.T) {
	cmd, args, noCtx := repl.ParseCommand("council --no-context")
	assert.Equal(t, "council", cmd)
	assert.Empty(t, args)
	assert.True(t, noCtx)
}

func TestParseCommand_ChainWithStages(t *testing.T) {
	cmd, args, noCtx := repl.ParseCommand("chain council ci-triage ticket")
	assert.Equal(t, "chain", cmd)
	assert.Equal(t, []string{"council", "ci-triage", "ticket"}, args)
	assert.False(t, noCtx)
}

func TestParseCommand_Empty(t *testing.T) {
	cmd, _, _ := repl.ParseCommand("  ")
	assert.Equal(t, "", cmd)
}

func TestParseCommand_Exit(t *testing.T) {
	cmd, _, _ := repl.ParseCommand("exit")
	assert.Equal(t, "exit", cmd)
}

func TestDispatchUnknownCommand(t *testing.T) {
	s := repl.NewSession()
	out, err := repl.DispatchCommand("nonexistent", []string{}, false, s, repl.DispatchConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
	assert.Empty(t, out)
}

func TestDispatchClearResetsSession(t *testing.T) {
	s := repl.NewSession()
	s.Append(makeResult("council"))
	out, err := repl.DispatchCommand("clear", []string{}, false, s, repl.DispatchConfig{})
	require.NoError(t, err)
	assert.Contains(t, out, "cleared")
	assert.Empty(t, s.Results())
}

func TestDispatchContextShowsAccumulated(t *testing.T) {
	s := repl.NewSession()
	s.Append(makeResult("council"))
	out, err := repl.DispatchCommand("context", []string{}, false, s, repl.DispatchConfig{})
	require.NoError(t, err)
	assert.Contains(t, out, "council")
}

func makeResult(stage string) chain.Result {
	return chain.Result{Stage: stage, Output: stage + " output"}
}
