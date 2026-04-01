// internal/providers/router.go
package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/89jobrien/devkit/internal/council"
	"github.com/89jobrien/devkit/internal/tools"
)

// TierOverrides allows .devkit.toml to pin specific model IDs per tier.
type TierOverrides struct {
	PrimaryProvider   string // "anthropic" | "openai" | "gemini"
	FastModel         string
	BalancedModel     string
	LargeContextModel string
	CodingModel       string
}

// RouterConfig holds API keys and optional overrides.
type RouterConfig struct {
	AnthropicKey string
	OpenAIKey    string
	GeminiKey    string
	Overrides    TierOverrides
}

// Router selects and chains providers for a given Tier.
type Router struct {
	cfg RouterConfig
}

// NewRouter constructs a Router from the given config.
func NewRouter(cfg RouterConfig) *Router {
	return &Router{cfg: cfg}
}

// roleTierMap maps council role keys to tiers.
var roleTierMap = map[string]Tier{
	"creative-explorer":   TierFast,
	"performance-analyst": TierFast,
	"general-analyst":     TierLargeContext,
	"security-reviewer":   TierBalanced,
	"strict-critic":       TierBalanced,
}

// TierForRole returns the appropriate Tier for a council role key.
// Unknown roles default to TierBalanced.
func TierForRole(role string) Tier {
	if t, ok := roleTierMap[role]; ok {
		return t
	}
	return TierBalanced
}

// For returns a council.Runner that tries the provider chain for the given tier.
// Providers with missing API keys are skipped. If no provider has a key,
// the runner returns an error on first call.
func (r *Router) For(tier Tier) council.Runner {
	return council.RunnerFunc(func(ctx context.Context, prompt string, toolNames []string) (string, error) {
		chain := r.chainFor(tier)
		if len(chain) == 0 {
			return "", errors.New("no provider available for tier " + string(tier) + ": set ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY")
		}
		var errs []string
		for _, entry := range chain {
			result, err := entry.provider.Chat(ctx, prompt)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", entry.name, err))
				continue
			}
			return result, nil
		}
		return "", fmt.Errorf("all providers failed for tier %s: %s", tier, strings.Join(errs, "; "))
	})
}

// AgentRunnerFor returns a council.Runner that passes tools through to the first
// AgentProvider in the chain. Use this for commands that require tool use (diagnose, review, meta).
func (r *Router) AgentRunnerFor(tier Tier, ts []tools.Tool) council.Runner {
	return council.RunnerFunc(func(ctx context.Context, prompt string, _ []string) (string, error) {
		chain := r.chainFor(tier)
		if len(chain) == 0 {
			return "", errors.New("no provider available for tier " + string(tier))
		}
		var errs []string
		for _, entry := range chain {
			ap, ok := entry.provider.(AgentProvider)
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: does not support tool use", entry.name))
				continue
			}
			result, err := ap.RunAgent(ctx, prompt, ts)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", entry.name, err))
				continue
			}
			return result, nil
		}
		return "", fmt.Errorf("no agent-capable provider available for tier %s: %s", tier, strings.Join(errs, "; "))
	})
}

type providerEntry struct {
	name     string
	provider ChatProvider
}

func (r *Router) chainFor(tier Tier) []providerEntry {
	var chain []providerEntry

	antModel, oaiModel, gemModel := r.modelsForTier(tier)

	// Anthropic first (primary for coding/balanced; supports tool use).
	if r.cfg.AnthropicKey != "" {
		chain = append(chain, providerEntry{
			name:     "anthropic/" + antModel,
			provider: NewAnthropicProvider(r.cfg.AnthropicKey, antModel, ""),
		})
	}
	// OpenAI second (supports tool use).
	if r.cfg.OpenAIKey != "" && oaiModel != "" {
		chain = append(chain, providerEntry{
			name:     "openai/" + oaiModel,
			provider: NewOpenAIProvider(r.cfg.OpenAIKey, oaiModel, ""),
		})
	}
	// Gemini last (large context / fast; no tool use).
	if r.cfg.GeminiKey != "" && gemModel != "" {
		chain = append(chain, providerEntry{
			name:     "gemini/" + gemModel,
			provider: NewGeminiProvider(r.cfg.GeminiKey, gemModel, ""),
		})
	}

	// If PrimaryProvider override is set, reorder to put that provider first.
	if r.cfg.Overrides.PrimaryProvider != "" {
		chain = reorderChain(chain, r.cfg.Overrides.PrimaryProvider)
	}
	return chain
}

func (r *Router) modelsForTier(tier Tier) (ant, oai, gem string) {
	ov := r.cfg.Overrides
	switch tier {
	case TierFast:
		ant = orDefault(ov.FastModel, ModelAnthropicFast)
		oai = ModelOpenAIFast
		gem = ModelGeminiFast
	case TierLargeContext:
		ant = orDefault(ov.LargeContextModel, ModelAnthropicLargeContext)
		oai = ModelOpenAIBalanced
		gem = ModelGeminiLargeContext
	case TierCoding:
		ant = orDefault(ov.CodingModel, ModelAnthropicCoding)
		oai = ModelOpenAICoding
		gem = "" // Gemini excluded from coding tier (no tool use)
	default: // TierBalanced
		ant = orDefault(ov.BalancedModel, ModelAnthropicBalanced)
		oai = ModelOpenAIBalanced
		gem = ModelGeminiBalanced
	}
	return
}

func orDefault(override, def string) string {
	if override != "" {
		return override
	}
	return def
}

func reorderChain(chain []providerEntry, primary string) []providerEntry {
	var first, rest []providerEntry
	for _, e := range chain {
		if strings.HasPrefix(e.name, primary) {
			first = append(first, e)
		} else {
			rest = append(rest, e)
		}
	}
	return append(first, rest...)
}
