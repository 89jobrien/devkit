// internal/repl/session_test.go
package repl_test

import (
	"testing"

	"github.com/89jobrien/devkit/internal/chain"
	"github.com/89jobrien/devkit/internal/repl"
	"github.com/stretchr/testify/assert"
)

func TestSessionAccumulatesResults(t *testing.T) {
	s := repl.NewSession()
	r1 := chain.Result{Stage: "council", Output: "council out"}
	r2 := chain.Result{Stage: "ticket", Output: "ticket out"}
	s.Append(r1)
	s.Append(r2)
	assert.Len(t, s.Results(), 2)
	assert.Equal(t, "council", s.Results()[0].Stage)
}

func TestSessionNoContextSkipsAccumulation(t *testing.T) {
	s := repl.NewSession()
	s.Append(chain.Result{Stage: "council", Output: "out"})
	// With --no-context, the result should NOT be appended.
	s.AppendIfContext(chain.Result{Stage: "ci-triage", Output: "ci out"}, false)
	assert.Len(t, s.Results(), 1)
	// With context (default), it is appended.
	s.AppendIfContext(chain.Result{Stage: "ticket", Output: "ticket out"}, true)
	assert.Len(t, s.Results(), 2)
}

func TestSessionContextSummary(t *testing.T) {
	s := repl.NewSession()
	s.Append(chain.Result{Stage: "council", Output: "council output text"})
	summary := s.ContextSummary()
	assert.Contains(t, summary, "council")
	assert.Contains(t, summary, "council output text")
}

func TestSessionClear(t *testing.T) {
	s := repl.NewSession()
	s.Append(chain.Result{Stage: "council", Output: "out"})
	s.Clear()
	assert.Empty(t, s.Results())
}
