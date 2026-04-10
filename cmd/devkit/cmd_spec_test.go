package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/ai/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSpecCmdRunsAllRoles(t *testing.T) {
	stub := spec.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		return "**Health Score:** 0.8\n**Summary**\nOK.", nil
	})

	dir := t.TempDir()
	specPath := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(specPath, []byte("# Test Spec\n\n## Problem\nSomething."), 0o644))

	cmd := newSpecCmd(stub, stub)
	cmd.SetArgs([]string{specPath})
	var out strings.Builder
	cmd.SetOut(&out)
	err := cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	body := out.String()
	for _, role := range []string{"completeness", "ambiguity", "scope", "critic", "creative", "generalist"} {
		assert.Contains(t, body, role, "output should contain role %q", role)
	}
	assert.Contains(t, body, "SYNTHESIS")
}

func TestNewSpecCmdAutoDiscovers(t *testing.T) {
	stub := spec.RunnerFunc(func(_ context.Context, _ string, _ []string) (string, error) {
		return "**Health Score:** 0.9\n**Summary**\nGood.", nil
	})

	dir := t.TempDir()
	specsDir := filepath.Join(dir, "docs", "superpowers", "specs")
	require.NoError(t, os.MkdirAll(specsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(specsDir, "foo.md"), []byte("# Foo"), 0o644))

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(orig) })

	cmd := newSpecCmd(stub, stub)
	var out strings.Builder
	cmd.SetOut(&out)
	err := cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	assert.Contains(t, out.String(), "SYNTHESIS")
}
