// cmd/devkit/runner.go
package main

import (
	"os"

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
