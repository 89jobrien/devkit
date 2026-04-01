package providers_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/providers"
	"github.com/89jobrien/devkit/internal/tools"
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
