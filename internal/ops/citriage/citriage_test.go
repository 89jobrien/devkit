package citriage_test

import (
	"context"
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
