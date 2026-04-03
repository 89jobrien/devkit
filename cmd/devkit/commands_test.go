// cmd/devkit/commands_test.go — CLI tests for the new AI subcommands.
// Uses stub runners injected via the constructor functions so no real LLM calls
// are made and no git history is required for file-mode tests.
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/changelog"
	"github.com/89jobrien/devkit/internal/explain"
	"github.com/89jobrien/devkit/internal/lint"
	"github.com/89jobrien/devkit/internal/pr"
	"github.com/89jobrien/devkit/internal/testgen"
	"github.com/89jobrien/devkit/internal/ticket"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRunner returns a runner that echoes its prompt back as the result.
func stubRunner(t *testing.T) func(ctx context.Context, prompt string, tools []string) (string, error) {
	t.Helper()
	return func(ctx context.Context, prompt string, tools []string) (string, error) {
		return "stub: " + prompt[:min(len(prompt), 80)], nil
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// runCmd executes a cobra command with the given args and returns stdout output.
func runCmd(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "devkit"}
	root.AddCommand(cmd)
	var buf bytes.Buffer
	root.SetOut(&buf)
	cmd.SetOut(&buf)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return buf.String(), err
}

// --- ticket ---

func TestTicketCmd_ArgInput(t *testing.T) {
	r := ticket.RunnerFunc(stubRunner(t))
	cmd := newTicketCmd(r)
	_, err := runCmd(t, cmd, "ticket", "fix the login bug")
	require.NoError(t, err)
}

func TestTicketCmd_FromFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(f, []byte("// TODO: fix memory leak\npackage main\n"), 0o644))

	r := ticket.RunnerFunc(stubRunner(t))
	cmd := newTicketCmd(r)
	_, err := runCmd(t, cmd, "ticket", "--from", f)
	require.NoError(t, err)
}

func TestTicketCmd_NoInput_Error(t *testing.T) {
	r := ticket.RunnerFunc(stubRunner(t))
	cmd := newTicketCmd(r)
	_, err := runCmd(t, cmd, "ticket")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provide a description")
}

func TestTicketCmd_FromFileMissing_Error(t *testing.T) {
	r := ticket.RunnerFunc(stubRunner(t))
	cmd := newTicketCmd(r)
	_, err := runCmd(t, cmd, "ticket", "--from", "/nonexistent/file.go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read")
}

func TestTicketCmd_RunnerError(t *testing.T) {
	r := ticket.RunnerFunc(func(ctx context.Context, prompt string, tools []string) (string, error) {
		return "", errors.New("llm unavailable")
	})
	cmd := newTicketCmd(r)
	_, err := runCmd(t, cmd, "ticket", "some description")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm unavailable")
}

// --- explain ---

func TestExplainCmd_FileMode(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "foo.go")
	require.NoError(t, os.WriteFile(f, []byte("package foo\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644))

	r := explain.RunnerFunc(stubRunner(t))
	cmd := newExplainCmd(r)
	_, err := runCmd(t, cmd, "explain", f)
	require.NoError(t, err)
}

func TestExplainCmd_FileMissing_Error(t *testing.T) {
	r := explain.RunnerFunc(stubRunner(t))
	cmd := newExplainCmd(r)
	_, err := runCmd(t, cmd, "explain", "/nonexistent/file.go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read")
}

func TestExplainCmd_DiffMode_InvalidRef_Error(t *testing.T) {
	r := explain.RunnerFunc(stubRunner(t))
	cmd := newExplainCmd(r)
	// Run in a temp dir with no git repo so validateRef always fails.
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(t.TempDir()))
	defer os.Chdir(orig)

	_, err := runCmd(t, cmd, "explain", "--base", "nonexistent-ref-abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in local repository")
}

// --- test-gen ---

func TestTestgenCmd_FileMode(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "calc.go")
	require.NoError(t, os.WriteFile(f, []byte("package calc\n\nfunc Mul(a, b int) int { return a * b }\n"), 0o644))

	r := testgen.RunnerFunc(stubRunner(t))
	cmd := newTestgenCmd(r)
	_, err := runCmd(t, cmd, "test-gen", f)
	require.NoError(t, err)
}

func TestTestgenCmd_FileMissing_Error(t *testing.T) {
	r := testgen.RunnerFunc(stubRunner(t))
	cmd := newTestgenCmd(r)
	_, err := runCmd(t, cmd, "test-gen", "/nonexistent/calc.go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read")
}

func TestTestgenCmd_DiffMode_InvalidRef_Error(t *testing.T) {
	r := testgen.RunnerFunc(stubRunner(t))
	cmd := newTestgenCmd(r)
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(t.TempDir()))
	defer os.Chdir(orig)

	_, err := runCmd(t, cmd, "test-gen", "--base", "nonexistent-ref-abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in local repository")
}

// --- changelog ---

func TestChangelogCmd_WithBase(t *testing.T) {
	// Use HEAD as the base — always resolvable in the devkit repo.
	r := changelog.RunnerFunc(stubRunner(t))
	cmd := newChangelogCmd(r)
	_, err := runCmd(t, cmd, "changelog", "--base", "HEAD")
	require.NoError(t, err)
}

func TestChangelogCmd_InvalidBase_Fallback(t *testing.T) {
	// Without --base, resolveChangelogBase() falls back to "main" or a tag.
	// We just verify the command doesn't panic when runner returns an error.
	r := changelog.RunnerFunc(func(ctx context.Context, prompt string, tools []string) (string, error) {
		return "", errors.New("runner error")
	})
	cmd := newChangelogCmd(r)
	_, err := runCmd(t, cmd, "changelog", "--base", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runner error")
}

// --- lint ---

func TestLintCmd_FileMode(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(f, []byte("package main\n\nfunc main() {}\n"), 0o644))

	r := lint.RunnerFunc(stubRunner(t))
	cmd := newLintCmd(r)
	_, err := runCmd(t, cmd, "lint", f)
	require.NoError(t, err)
}

func TestLintCmd_FileMissing_Error(t *testing.T) {
	r := lint.RunnerFunc(stubRunner(t))
	cmd := newLintCmd(r)
	_, err := runCmd(t, cmd, "lint", "/nonexistent/file.go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read")
}

func TestLintCmd_NoArgs_Error(t *testing.T) {
	r := lint.RunnerFunc(stubRunner(t))
	cmd := newLintCmd(r)
	_, err := runCmd(t, cmd, "lint")
	require.Error(t, err)
}

// --- pr ---

func TestPrCmd_InvalidBase_Error(t *testing.T) {
	r := pr.RunnerFunc(stubRunner(t))
	cmd := newPrCmd(r)
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(t.TempDir()))
	defer os.Chdir(orig)

	_, err := runCmd(t, cmd, "pr", "--base", "nonexistent-ref-abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in local repository")
}

func TestPrCmd_ValidBase(t *testing.T) {
	r := pr.RunnerFunc(stubRunner(t))
	cmd := newPrCmd(r)
	// HEAD is always a valid ref in our repo.
	_, err := runCmd(t, cmd, "pr", "--base", "HEAD")
	require.NoError(t, err)
}

func TestPrCmd_RunnerError(t *testing.T) {
	r := pr.RunnerFunc(func(ctx context.Context, prompt string, tools []string) (string, error) {
		return "", errors.New("model timeout")
	})
	cmd := newPrCmd(r)
	_, err := runCmd(t, cmd, "pr", "--base", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model timeout")
}

// --- command registration ---

func TestAllNewCommandsRegistered(t *testing.T) {
	root := &cobra.Command{Use: "devkit"}
	root.AddCommand(
		newPrCmd(nil),
		newChangelogCmd(nil),
		newLintCmd(nil),
		newExplainCmd(nil),
		newTestgenCmd(nil),
		newTicketCmd(nil),
	)
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"pr", "changelog", "lint", "explain", "test-gen", "ticket"} {
		assert.True(t, names[want], "command %q not registered", want)
	}
}

// --- ticket input precedence ---

func TestTicketCmd_ArgTakesPrecedenceOverFrom(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(f, []byte("// TODO: fix me\n"), 0o644))

	var capturedPrompt string
	r := ticket.RunnerFunc(func(ctx context.Context, prompt string, tools []string) (string, error) {
		capturedPrompt = prompt
		return "ok", nil
	})
	cmd := newTicketCmd(r)
	// When both arg and --from are given, arg wins (from is only used when prompt is empty).
	// The full prompt sent to the runner includes the user request embedded in the ticket template.
	_, err := runCmd(t, cmd, "ticket", "fix the login bug", "--from", f)
	require.NoError(t, err)
	assert.Contains(t, capturedPrompt, "fix the login bug", "arg input should be used")
	assert.NotContains(t, capturedPrompt, "Find TODOs", "file-mode prompt should not be used when arg is present")
}

func TestTicketCmd_FromUsedWhenNoArg(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(f, []byte("// TODO: fix memory leak\n"), 0o644))

	var capturedPrompt string
	r := ticket.RunnerFunc(func(ctx context.Context, prompt string, tools []string) (string, error) {
		capturedPrompt = prompt
		return "ok", nil
	})
	cmd := newTicketCmd(r)
	_, err := runCmd(t, cmd, "ticket", "--from", f)
	require.NoError(t, err)
	assert.True(t, strings.Contains(capturedPrompt, "Find TODOs"), "expected file-mode prompt, got: %s", capturedPrompt)
	assert.True(t, strings.Contains(capturedPrompt, f), "expected file path in prompt")
}
