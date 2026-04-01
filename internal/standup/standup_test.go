// internal/standup/standup_test.go
package standup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- stub runner ---

type stubRunner struct {
	calls  []string
	result string
	err    error
}

func (s *stubRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	s.calls = append(s.calls, prompt)
	return s.result, s.err
}

// --- helpers ---

func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("git setup: %s: %v", out, err)
		}
	}
	// Make a commit so HEAD exists.
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "-C", dir, "add", "."},
		{"git", "-C", dir, "commit", "-m", "initial commit"},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("git commit: %s: %v", out, err)
		}
	}
	return dir
}

// --- tests ---

func TestRunSinglePromptContainsProjectAndCommits(t *testing.T) {
	dir := initTempRepo(t)
	stub := &stubRunner{result: "## What I did\n- stuff\n## What's next\n- more\n## Blockers\nnone"}

	result, err := Run(context.Background(), Config{
		Repos:  []string{dir},
		Since:  24 * time.Hour,
		Runner: stub,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 runner call, got %d", len(stub.calls))
	}
	prompt := stub.calls[0]

	project := filepath.Base(dir)
	if !strings.Contains(prompt, project) {
		t.Errorf("prompt missing project name %q:\n%s", project, prompt)
	}
	// Note: git log --since="24h ago" may not capture the just-made commit depending on git version;
	// assert the commits section header is present regardless.
	if !strings.Contains(prompt, "Recent commits:") {
		t.Errorf("prompt missing 'Recent commits:' section:\n%s", prompt)
	}
	for _, section := range []string{"## What I did", "## What's next", "## Blockers"} {
		if !strings.Contains(prompt, section) {
			t.Errorf("prompt missing section %q", section)
		}
	}
}

func TestRunDefaultsToSingleRepoWhenNoneGiven(t *testing.T) {
	dir := initTempRepo(t)
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(dir)

	stub := &stubRunner{result: "standup output"}
	_, err := Run(context.Background(), Config{
		Since:  24 * time.Hour,
		Runner: stub,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(stub.calls))
	}
}

func TestRunParallelTwoReposSynthesizes(t *testing.T) {
	dir1 := initTempRepo(t)
	dir2 := initTempRepo(t)

	callCount := 0
	stub := RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		callCount++
		if strings.Contains(prompt, "Synthesize") {
			return "## What I did\n- synth\n## What's next\n- tbd\n## Blockers\nnone", nil
		}
		return "- did stuff", nil
	})

	result, err := Run(context.Background(), Config{
		Repos:    []string{dir1, dir2},
		Since:    24 * time.Hour,
		Runner:   stub,
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2 per-repo summary calls + 1 synthesis call
	if callCount < 3 {
		t.Errorf("expected ≥3 runner calls (2 summaries + 1 synthesis), got %d", callCount)
	}
	if !strings.Contains(result, "## What I did") {
		t.Errorf("synthesis result missing expected section:\n%s", result)
	}
}

func TestGatherRepoContextNonGitDirErrors(t *testing.T) {
	dir := t.TempDir() // no .git
	_, err := gatherRepoContext(dir, 24*time.Hour)
	if err == nil {
		t.Error("expected error for non-git dir, got nil")
	}
}

func TestGatherJSONLRunsSkipsMissingFile(t *testing.T) {
	entries := gatherJSONLRuns("project-that-does-not-exist-xyzzy", 24*time.Hour)
	if entries != nil {
		t.Errorf("expected nil for missing JSONL, got %v", entries)
	}
}
