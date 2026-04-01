package main

import (
	"testing"

	"github.com/89jobrien/devkit/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRouterFromConfig_NoKeys(t *testing.T) {
	cfg := &Config{}
	r, err := newRouterFromConfig(cfg)
	require.NoError(t, err)
	assert.NotNil(t, r)
}

func TestNewRouterFromConfig_WithOverrides(t *testing.T) {
	cfg := &Config{}
	cfg.Providers.Primary = "openai"
	cfg.Providers.CodingModel = "gpt-5.4-custom"
	r, err := newRouterFromConfig(cfg)
	require.NoError(t, err)
	assert.NotNil(t, r)
	_ = r.For(providers.TierCoding) // must not panic
}
