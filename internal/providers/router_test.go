package providers_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouterForTier_AnthropicOnlyKey(t *testing.T) {
	r := providers.NewRouter(providers.RouterConfig{
		AnthropicKey: "ant-key",
	})
	runner := r.For(providers.TierCoding)
	require.NotNil(t, runner)
}

func TestRouterForTier_NoKeysReturnsError(t *testing.T) {
	r := providers.NewRouter(providers.RouterConfig{})
	runner := r.For(providers.TierFast)
	_, err := runner.Run(context.Background(), "hello", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider available")
}

func TestRouterTierForRole(t *testing.T) {
	assert.Equal(t, providers.TierFast, providers.TierForRole("creative-explorer"))
	assert.Equal(t, providers.TierFast, providers.TierForRole("performance-analyst"))
	assert.Equal(t, providers.TierLargeContext, providers.TierForRole("general-analyst"))
	assert.Equal(t, providers.TierBalanced, providers.TierForRole("strict-critic"))
	assert.Equal(t, providers.TierBalanced, providers.TierForRole("unknown-role"))
}

func TestRouterConfigOverride(t *testing.T) {
	// When .devkit.toml sets fast_model, the router uses it.
	r := providers.NewRouter(providers.RouterConfig{
		AnthropicKey: "ant-key",
		Overrides: providers.TierOverrides{
			FastModel: "claude-haiku-4-5-custom",
		},
	})
	// Just verify it constructs without panic — model used at call time.
	assert.NotNil(t, r.For(providers.TierFast))
}
