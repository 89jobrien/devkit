package providers_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/ai/providers"
	"github.com/89jobrien/devkit/internal/infra/tools"
	"github.com/stretchr/testify/assert"
)

// Compile-time checks: stub types must satisfy the interfaces.
type stubChat struct{}

func (s stubChat) Chat(_ context.Context, _ string) (string, error) { return "ok", nil }

type stubAgent struct{ stubChat }

func (s stubAgent) RunAgent(_ context.Context, _ string, _ []tools.Tool) (string, error) {
	return "ok", nil
}

func TestInterfacesCompile(t *testing.T) {
	var _ providers.ChatProvider = stubChat{}
	var _ providers.AgentProvider = stubAgent{}
}

func TestModelConstants(t *testing.T) {
	assert.NotEmpty(t, providers.ModelAnthropicFast)
	assert.NotEmpty(t, providers.ModelAnthropicBalanced)
	assert.NotEmpty(t, providers.ModelAnthropicLargeContext)
	assert.NotEmpty(t, providers.ModelAnthropicCoding)
	assert.NotEmpty(t, providers.ModelOpenAIFast)
	assert.NotEmpty(t, providers.ModelOpenAIBalanced)
	assert.NotEmpty(t, providers.ModelOpenAICoding)
	assert.NotEmpty(t, providers.ModelGeminiFast)
	assert.NotEmpty(t, providers.ModelGeminiBalanced)
	assert.NotEmpty(t, providers.ModelGeminiLargeContext)
}
