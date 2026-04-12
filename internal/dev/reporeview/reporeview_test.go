package reporeview_test

import (
	"context"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/dev/reporeview"
)


type stubRunner struct{ response string }

func (s *stubRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	return s.response, nil
}

func TestRunRequiresRunner(t *testing.T) {
	_, err := reporeview.Run(context.Background(), reporeview.Config{RepoPath: t.TempDir()})
	if err == nil {
		t.Fatal("expected error when runner is nil")
	}
}

func TestRunCallsRunner(t *testing.T) {
	runner := &stubRunner{response: "top issue: no tests"}
	result, err := reporeview.Run(context.Background(), reporeview.Config{
		RepoPath: t.TempDir(),
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "top issue") {
		t.Errorf("expected runner response, got: %s", result)
	}
}

func TestRunPromptContainsRepoName(t *testing.T) {
	var capturedPrompt string
	runner := &captureRunner{}
	runner.fn = func(prompt string) {
		capturedPrompt = prompt
	}
	dir := t.TempDir()
	_, _ = reporeview.Run(context.Background(), reporeview.Config{
		RepoPath: dir,
		Runner:   runner,
	})
	if !strings.Contains(capturedPrompt, "reviewing the repository") {
		t.Errorf("expected review framing in prompt, got: %s", capturedPrompt)
	}
}

func TestRunJSONFormat(t *testing.T) {
	runner := &stubRunner{response: "top issue: no tests"}
	result, err := reporeview.Run(context.Background(), reporeview.Config{
		RepoPath: t.TempDir(),
		Runner:   runner,
		Format:   "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result, "{") {
		t.Errorf("expected JSON output, got: %s", result)
	}
	if !strings.Contains(result, `"output"`) {
		t.Errorf("expected output key in JSON, got: %s", result)
	}
}

func TestRunUnknownFormat(t *testing.T) {
	runner := &stubRunner{response: "out"}
	_, err := reporeview.Run(context.Background(), reporeview.Config{
		RepoPath: t.TempDir(),
		Runner:   runner,
		Format:   "xml",
	})
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
	if !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("expected 'unknown format' in error, got: %v", err)
	}
}

func TestRunJSONFormatCaseInsensitive(t *testing.T) {
	runner := &stubRunner{response: "top issue: no tests"}
	result, err := reporeview.Run(context.Background(), reporeview.Config{
		RepoPath: t.TempDir(),
		Runner:   runner,
		Format:   "JSON",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result, "{") {
		t.Errorf("expected JSON output for uppercase format, got: %s", result)
	}
}

type captureRunner struct {
	fn func(string)
}

func (c *captureRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	if c.fn != nil {
		c.fn(prompt)
	}
	return "ok", nil
}
