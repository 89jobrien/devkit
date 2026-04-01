package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigProviderOverrides(t *testing.T) {
	dir := t.TempDir()
	toml := `
[providers]
primary = "openai"
coding_model = "gpt-5.4-custom"
fast_model = "gemini-3-flash-preview"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".devkit.toml"), []byte(toml), 0o644))
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(orig)

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.Providers.Primary)
	assert.Equal(t, "gpt-5.4-custom", cfg.Providers.CodingModel)
	assert.Equal(t, "gemini-3-flash-preview", cfg.Providers.FastModel)
}
