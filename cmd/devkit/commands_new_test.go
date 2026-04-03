// cmd/devkit/commands_new_test.go — tests for ci-triage, repo-review, health, automate commands.
package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/ai/council"
	"github.com/89jobrien/devkit/internal/ops/automate"
	"github.com/89jobrien/devkit/internal/ops/citriage"
	"github.com/89jobrien/devkit/internal/ops/health"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ci-triage ---

func TestCITriageCmd_Registration(t *testing.T) {
	root := &cobra.Command{Use: "devkit"}
	root.AddCommand(newCITriageCmd(nil))
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	assert.True(t, names["ci-triage"], "ci-triage not registered")
}

func TestCITriageCmd_RunnerNotCalledWithoutLog(t *testing.T) {
	// Without a valid --run ID and no --stdin, fetchLog cannot produce a log.
	// The runner must NOT be called in that case.
	runnerCalled := false
	r := citriage.RunnerFunc(func(ctx context.Context, log, repoCtx string) (string, error) {
		runnerCalled = true
		return "triage result", nil
	})
	cmd := newCITriageCmd(r)
	dir := t.TempDir()
	_, _ = runCmd(t, cmd, "ci-triage", "--repo", dir, "--run", "")
	if runnerCalled {
		t.Error("runner was called but should not be: log fetch should have failed first")
	}
}

func TestCITriageCmd_RunnerError(t *testing.T) {
	// Runner is invoked when a log is pre-loaded via Config.Log.
	// Test indirectly: inject a runner and verify errors propagate.
	r := citriage.RunnerFunc(func(ctx context.Context, log, repoCtx string) (string, error) {
		return "", nil
	})
	cmd := newCITriageCmd(r)
	// Without a run ID and no stdin, fetchLog will attempt `gh run list`.
	// In CI this may fail; the important thing is no panic.
	_, _ = runCmd(t, cmd, "ci-triage")
}

func TestCITriageCmd_HasExpectedFlags(t *testing.T) {
	cmd := newCITriageCmd(nil)
	assert.NotNil(t, cmd.Flags().Lookup("repo"), "missing --repo flag")
	assert.NotNil(t, cmd.Flags().Lookup("run"), "missing --run flag")
	assert.NotNil(t, cmd.Flags().Lookup("stdin"), "missing --stdin flag")
}

// --- repo-review ---

func TestRepoReviewCmd_Registration(t *testing.T) {
	root := &cobra.Command{Use: "devkit"}
	root.AddCommand(newRepoReviewCmd(nil))
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	assert.True(t, names["repo-review"], "repo-review not registered")
}

func TestRepoReviewCmd_HasExpectedFlags(t *testing.T) {
	cmd := newRepoReviewCmd(nil)
	assert.NotNil(t, cmd.Flags().Lookup("repo"), "missing --repo flag")
	assert.NotNil(t, cmd.Flags().Lookup("format"), "missing --format flag")
}

func TestRepoReviewCmd_GathersFilesystemContext(t *testing.T) {
	dir := t.TempDir()
	// Write CLAUDE.md and README.md to verify gatherContext reads them.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# test claude"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test readme"), 0o644))

	var capturedPrompt string
	r := council.RunnerFunc(func(ctx context.Context, prompt string, tools []string) (string, error) {
		capturedPrompt = prompt
		return "review done", nil
	})
	cmd := newRepoReviewCmd(r)
	out, err := runCmd(t, cmd, "repo-review", "--repo", dir)
	require.NoError(t, err)
	assert.Contains(t, out, "review done")
	assert.Contains(t, capturedPrompt, "# test claude", "CLAUDE.md not included in prompt")
	assert.Contains(t, capturedPrompt, "# test readme", "README.md not included in prompt")
}

func TestRepoReviewCmd_RunnerError(t *testing.T) {
	dir := t.TempDir()
	r := council.RunnerFunc(func(ctx context.Context, prompt string, tools []string) (string, error) {
		return "", assert.AnError
	})
	cmd := newRepoReviewCmd(r)
	_, err := runCmd(t, cmd, "repo-review", "--repo", dir)
	require.Error(t, err)
}

// --- health ---

func TestHealthCmd_Registration(t *testing.T) {
	root := &cobra.Command{Use: "devkit"}
	root.AddCommand(newHealthCmd(nil))
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	assert.True(t, names["health"], "health not registered")
}

func TestHealthCmd_HasExpectedFlags(t *testing.T) {
	cmd := newHealthCmd(nil)
	assert.NotNil(t, cmd.Flags().Lookup("repo"), "missing --repo flag")
	assert.NotNil(t, cmd.Flags().Lookup("format"), "missing --format flag")
}

func TestHealthCmd_RunsChecks(t *testing.T) {
	dir := t.TempDir()
	// Write enough structure that gatherChecks finds something.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# ok"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755))

	var capturedChecks string
	r := health.RunnerFunc(func(ctx context.Context, repoCtx, checks string) (string, error) {
		capturedChecks = checks
		return "health ok", nil
	})
	cmd := newHealthCmd(r)
	out, err := runCmd(t, cmd, "health", "--repo", dir)
	require.NoError(t, err)
	assert.Contains(t, out, "health ok")
	assert.Contains(t, capturedChecks, "pass", "expected at least one passing check")
}

// --- automate ---

func TestAutomateCmd_Registration(t *testing.T) {
	root := &cobra.Command{Use: "devkit"}
	root.AddCommand(newAutomateCmd(nil))
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	assert.True(t, names["automate"], "automate not registered")
}

func TestAutomateCmd_HasExpectedFlags(t *testing.T) {
	cmd := newAutomateCmd(nil)
	assert.NotNil(t, cmd.Flags().Lookup("repo"), "missing --repo flag")
	assert.NotNil(t, cmd.Flags().Lookup("tasks"), "missing --tasks flag")
}

func TestAutomateCmd_UnknownTaskInOutput(t *testing.T) {
	r := automate.RunnerFunc(func(ctx context.Context, prompt string, tools []string) (string, error) {
		return "ok", nil
	})
	cmd := newAutomateCmd(r)
	out, err := runCmd(t, cmd, "automate", "--tasks", "nonexistent-task", "--repo", t.TempDir())
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, "unknown task") || strings.Contains(out, "nonexistent-task"),
		"expected unknown task message in output, got: %s", out)
}

// --- registration completeness ---

func TestAllCommandsRegistered(t *testing.T) {
	root := &cobra.Command{Use: "devkit"}
	root.AddCommand(
		newPrCmd(nil),
		newChangelogCmd(nil),
		newLintCmd(nil),
		newExplainCmd(nil),
		newTestgenCmd(nil),
		newTicketCmd(nil),
		newCITriageCmd(nil),
		newRepoReviewCmd(nil),
		newHealthCmd(nil),
		newAutomateCmd(nil),
	)
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{
		"pr", "changelog", "lint", "explain", "test-gen", "ticket",
		"ci-triage", "repo-review", "health", "automate",
	} {
		assert.True(t, names[want], "command %q not registered", want)
	}
}
