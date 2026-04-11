// internal/chain/result.go
package chain

// Result is the universal envelope for a single pipeline stage output.
// Skipped stages have zero-value Result with Stage == "".
// Payload carries the stage-specific typed struct; use a type switch to inspect it.
// Meta holds lightweight k/v for fields that don't warrant a dedicated struct.
type Result struct {
	Stage   string         // stage name, e.g. "council"
	Output  string         // rendered string output (always populated if not skipped)
	Payload any            // typed: *CouncilPayload, *CITriagePayload, etc.
	Err     error          // non-nil if stage failed; Output may still be partially populated
	Meta    map[string]any // lightweight k/v for unstructured metadata
}

// IsSkipped returns true when this slot represents a stage not selected by the user.
func (r Result) IsSkipped() bool {
	return r.Stage == "" && r.Output == "" && r.Err == nil
}

// --- Typed payload structs ---

// CouncilPayload carries structured output from the council stage.
type CouncilPayload struct {
	HealthScore float64           // meta-score average across roles (0–1)
	RoleOutputs map[string]string // role name → full role output text
}

// CITriagePayload carries structured output from the ci-triage stage.
type CITriagePayload struct {
	RootCause   string
	Suggestions []string
	LogSnippet  string // the filtered log sent to the runner
}

// LogPatternPayload carries structured output from the log-pattern stage.
type LogPatternPayload struct {
	Patterns []string // recurring error patterns found
	Count    int
}

// DiagnosePayload carries structured output from the diagnose stage.
type DiagnosePayload struct {
	Summary     string
	Severity    string // "low" | "medium" | "high" | "critical"
	NextActions []string
}

// TicketPayload carries structured output from the ticket stage.
type TicketPayload struct {
	Title  string
	Body   string
	Labels []string
}

// PRPayload carries structured output from the pr stage.
type PRPayload struct {
	Title string
	Body  string
}

// SynthesisPayload carries the final gpt-5.4 synthesis output.
type SynthesisPayload struct {
	Summary     string
	KeyFindings []string
	NextActions []string
}
