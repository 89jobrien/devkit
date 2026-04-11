// internal/repl/session.go
package repl

import (
	"fmt"
	"strings"

	"github.com/89jobrien/devkit/internal/chain"
)

// Session holds accumulated results across REPL commands.
// Context accumulation is on by default; --no-context opts out per-command.
type Session struct {
	results []chain.Result
}

// NewSession constructs an empty Session.
func NewSession() *Session { return &Session{} }

// Append always adds the result regardless of --no-context.
func (s *Session) Append(r chain.Result) {
	s.results = append(s.results, r)
}

// AppendIfContext adds the result only when useContext is true (default behavior).
// Pass useContext=false when the command was run with --no-context.
func (s *Session) AppendIfContext(r chain.Result, useContext bool) {
	if useContext {
		s.results = append(s.results, r)
	}
}

// Results returns a copy of all accumulated results.
func (s *Session) Results() []chain.Result {
	out := make([]chain.Result, len(s.results))
	copy(out, s.results)
	return out
}

// ContextSummary returns a markdown string of all session results suitable for
// injecting into prompt context for the next command.
func (s *Session) ContextSummary() string {
	if len(s.results) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Session context\n\n")
	for _, r := range s.results {
		if r.IsSkipped() {
			continue
		}
		if r.Err != nil {
			fmt.Fprintf(&sb, "### %s (failed: %v)\n\n", r.Stage, r.Err)
			continue
		}
		fmt.Fprintf(&sb, "### %s\n\n%s\n\n", r.Stage, r.Output)
	}
	return sb.String()
}

// Clear removes all accumulated results.
func (s *Session) Clear() { s.results = nil }
