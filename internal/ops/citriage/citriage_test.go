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

// TestRunFilteringBeforeTruncation verifies that filterLog runs before the
// 64KB cap and before dispatch to the runner. It feeds a log that is large
// enough to require truncation only if boilerplate is NOT removed, and
// asserts the runner receives cleaned output with the truncation marker.
func TestRunFilteringBeforeTruncation(t *testing.T) {
	var capturedLog string
	runner := citriage.RunnerFunc(func(_ context.Context, log, _ string) (string, error) {
		capturedLog = log
		return "ok", nil
	})

	// Build a log: 800 boilerplate lines (~80KB raw) + a real error line.
	// After filtering, the boilerplate is gone and the error fits under 64KB.
	var b strings.Builder
	for i := 0; i < 800; i++ {
		b.WriteString("build\tSetup\t2026-01-01T00:00:00.0000000Z ##[group]Run actions/checkout@v4\n")
		b.WriteString("build\tSetup\t2026-01-01T00:00:00.0000000Z Runner Image Provisioner v1.2.3\n")
	}
	b.WriteString("build\tBuild\t2026-01-01T00:00:01.0000000Z ./main.go:10:3: undefined: foo\n")
	raw := b.String()

	_, err := citriage.Run(context.Background(), citriage.Config{
		RepoPath: t.TempDir(),
		Log:      raw,
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Boilerplate must be gone — filtering happened before truncation.
	if strings.Contains(capturedLog, "Runner Image Provisioner") {
		t.Error("boilerplate reached the runner — filter did not run before dispatch")
	}
	if strings.Contains(capturedLog, "##[group]") {
		t.Error("##[group] boilerplate reached the runner")
	}
	// The real error must survive.
	if !strings.Contains(capturedLog, "undefined: foo") {
		t.Errorf("error line was lost; runner received: %q", capturedLog[:min(len(capturedLog), 200)])
	}
	// The log must NOT be truncated — filtering reduced it below 64KB.
	if strings.Contains(capturedLog, "[truncated]") {
		t.Error("log was truncated despite filtering; filter may not have run before the cap")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
