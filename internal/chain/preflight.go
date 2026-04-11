// internal/chain/preflight.go
package chain

import (
	"fmt"
	"os"
)

// PreflightConfig holds everything Preflight needs to validate.
type PreflightConfig struct {
	Stages       []string // user-requested stage names
	AnthropicKey string
	OpenAIKey    string
	RepoPath     string
	// LookupBinary checks whether a binary is on PATH. Overridable in tests.
	LookupBinary func(name string) bool
}

// binaryRequirements maps stage names to required binaries.
var binaryRequirements = map[string][]string{
	"ci-triage": {"gh"},
}

// Preflight validates env, stage names, repo path, and binary requirements.
// Returns all failures at once — never stops at the first error.
func Preflight(cfg PreflightConfig) []error {
	var errs []error

	// Validate stage names via SelectStages.
	if _, err := SelectStages(cfg.Stages); err != nil {
		errs = append(errs, err)
	}

	// At least one LLM key must be present (synthesis always needs OpenAI).
	if cfg.AnthropicKey == "" && cfg.OpenAIKey == "" {
		errs = append(errs, fmt.Errorf("preflight: at least one API key required (ANTHROPIC_API_KEY or OPENAI_API_KEY)"))
	}
	// Synthesis always uses OpenAI gpt-5.4.
	if cfg.OpenAIKey == "" {
		errs = append(errs, fmt.Errorf("preflight: OPENAI_API_KEY required for synthesis stage"))
	}

	// Repo path must exist if provided.
	if cfg.RepoPath != "" {
		if _, err := os.Stat(cfg.RepoPath); err != nil {
			errs = append(errs, fmt.Errorf("preflight: repo path %q not found: %w", cfg.RepoPath, err))
		}
	}

	// Binary requirements per stage.
	lookup := cfg.LookupBinary
	if lookup == nil {
		lookup = defaultLookup
	}
	for _, stage := range cfg.Stages {
		for _, bin := range binaryRequirements[stage] {
			if !lookup(bin) {
				errs = append(errs, fmt.Errorf("preflight: stage %q requires %q on PATH", stage, bin))
			}
		}
	}

	return errs
}

func defaultLookup(name string) bool {
	_, err := lookPath(name)
	return err == nil
}
