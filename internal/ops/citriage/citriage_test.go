package citriage_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/ops/citriage"
)

type stubRunner struct{ response string }

func (s *stubRunner) Run(_ context.Context, log, repoContext string) (string, error) {
	return s.response, nil
}

func TestRunRequiresRunner(t *testing.T) {
	_, err := citriage.Run(context.Background(), citriage.Config{
		RepoPath: t.TempDir(),
		Log:      "some log",
	})
	if err == nil {
		t.Fatal("expected error when runner is nil")
	}
}

func TestRunWithPreloadedLog(t *testing.T) {
	runner := &stubRunner{response: "root_cause: compile error"}
	result, err := citriage.Run(context.Background(), citriage.Config{
		RepoPath: t.TempDir(),
		Log:      "FAILED: compilation error in main.go",
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "root_cause") {
		t.Errorf("expected runner response in result, got: %s", result)
	}
}

// TestFetchLogRunListUsesRepoDir verifies that the gh run list command is
// executed with cmd.Dir set to repoPath (repo-scoping fix for devkit-10).
func TestFetchLogRunListUsesRepoDir(t *testing.T) {
	repoDir := t.TempDir()

	// Create a fake gh script that records its working directory.
	recordFile := repoDir + "/recorded_dir"
	fakeGh := repoDir + "/gh"
	script := "#!/bin/sh\npwd > " + recordFile + "\necho 99999999\n"
	if err := os.WriteFile(fakeGh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// Inject the fake gh via the package-level seam.
	restore := citriage.SetGhBinary(fakeGh)
	defer restore()

	runner := &stubRunner{response: "triage result"}
	_, err := citriage.Run(context.Background(), citriage.Config{
		RepoPath: repoDir,
		// No Log, no RunID — forces gh run list to be called
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recorded, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatalf("fake gh did not write recorded_dir: %v", err)
	}
	gotDir := strings.TrimSpace(string(recorded))
	if gotDir != repoDir {
		t.Errorf("gh run list ran in %q, want %q", gotDir, repoDir)
	}
}

func TestFetchLogRunListUsesRepoDirWithExplicitRunID(t *testing.T) {
	// When RunID is provided, only gh run view is called; verify it also uses repoDir.
	repoDir := t.TempDir()
	recordFile := repoDir + "/recorded_dir"
	fakeGh := repoDir + "/gh"
	script := "#!/bin/sh\npwd > " + recordFile + "\necho 'some log output'\n"
	if err := os.WriteFile(fakeGh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	restore := citriage.SetGhBinary(fakeGh)
	defer restore()

	runner := &stubRunner{response: "triage result"}
	_, err := citriage.Run(context.Background(), citriage.Config{
		RepoPath: repoDir,
		RunID:    "99999999",
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recorded, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatalf("fake gh did not write recorded_dir: %v", err)
	}
	gotDir := strings.TrimSpace(string(recorded))
	if gotDir != repoDir {
		t.Errorf("gh run view ran in %q, want %q", gotDir, repoDir)
	}
}

func TestLogTruncation(t *testing.T) {
	var capturedLog string
	runner := citriage.RunnerFunc(func(_ context.Context, log, _ string) (string, error) {
		capturedLog = log
		return "ok", nil
	})
	bigLog := strings.Repeat("x", 70*1024) // 70KB > 64KB limit
	_, err := citriage.Run(context.Background(), citriage.Config{
		RepoPath: t.TempDir(),
		Log:      bigLog,
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(capturedLog, "[truncated]") {
		t.Errorf("expected log to be truncated, got len=%d", len(capturedLog))
	}
}
