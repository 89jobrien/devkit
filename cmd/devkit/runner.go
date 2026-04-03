// cmd/devkit/runner.go
package main

import (
	"fmt"
	"io"
	"os"
	"time"

	devlog "github.com/89jobrien/devkit/internal/log"
	"github.com/89jobrien/devkit/internal/providers"
)

// newRouterFromConfig constructs a Router using API keys from the environment
// and any model overrides from .devkit.toml.
func newRouterFromConfig(cfg *Config) (*providers.Router, error) {
	return providers.NewRouter(providers.RouterConfig{
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIKey:    os.Getenv("OPENAI_API_KEY"),
		GeminiKey:    os.Getenv("GEMINI_API_KEY"),
		Overrides: providers.TierOverrides{
			PrimaryProvider:   cfg.Providers.Primary,
			FastModel:         cfg.Providers.FastModel,
			BalancedModel:     cfg.Providers.BalancedModel,
			LargeContextModel: cfg.Providers.LargeContextModel,
			CodingModel:       cfg.Providers.CodingModel,
		},
	}), nil
}

// setupRunner loads config, sets DEVKIT_PROJECT, and returns a runner for the
// given tier. Used by command constructors when no injected runner is provided.
func setupRunner(tier providers.Tier) (func(string) string, *providers.Router, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, nil, err
	}
	if cfg.Project.Name != "" {
		os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
	}
	router, err := newRouterFromConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	// Return project name setter and router; caller wraps into concrete RunnerFunc.
	return func(name string) string {
		if name == "" {
			return cfg.Project.Name
		}
		return name
	}, router, nil
}

// buildTierRunner is a convenience used by the many simple commands that just
// need a council-compatible runner at a fixed tier.
func buildTierRunner(tier providers.Tier) (providers.Runner, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	if cfg.Project.Name != "" {
		os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
	}
	router, err := newRouterFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	return router.For(tier), nil
}

// logResult writes result to w, records devlog.Complete, and persists via
// SaveCommitLog. Errors from SaveCommitLog are surfaced as warnings to stderr.
func logResult(w io.Writer, command string, sha string, logMeta map[string]string, result string, id devlog.RunID, start time.Time) {
	fmt.Fprintln(w, result)
	devlog.Complete(id, command, logMeta, result, time.Since(start))
	if path, err := devlog.SaveCommitLog(sha, command, result, logMeta); err != nil {
		fmt.Fprintf(os.Stderr, "devkit: warning: failed to save log: %v\n", err)
	} else {
		fmt.Fprintf(w, "\nLogged to: %s\n", path)
	}
}
