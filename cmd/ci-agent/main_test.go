// cmd/ci-agent/main_test.go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"content":[{"type":"text","text":"root cause: missing import"}]}`))
	}))
	defer srv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "test")
	text, name, err := askWithFallback("diagnose this", srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "root cause: missing import", text)
	assert.Contains(t, name, "anthropic")
}

func TestProviderFallbackSkipsWhenNoKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	_, _, err := askWithFallback("diagnose this")
	assert.ErrorIs(t, err, errDiagnosisUnavailable)
}
