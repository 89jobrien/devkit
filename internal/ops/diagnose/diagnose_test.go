// internal/diagnose/diagnose_test.go
package diagnose_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/89jobrien/devkit/internal/ops/diagnose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRunner struct {
	response       string
	capturedTools  []string
	capturedPrompt string
}

func (s *stubRunner) Run(_ context.Context, prompt string, tools []string) (string, error) {
	s.capturedPrompt = prompt
	s.capturedTools = tools
	return s.response, nil
}

func TestRunReturnsOutput(t *testing.T) {
	r := &stubRunner{response: "Root cause: disk full."}
	result, err := diagnose.Run(context.Background(), diagnose.Config{
		Runner: r,
	})
	require.NoError(t, err)
	assert.Equal(t, "Root cause: disk full.", result)
}

func TestRunRequestsBashTool(t *testing.T) {
	r := &stubRunner{response: "ok"}
	_, _ = diagnose.Run(context.Background(), diagnose.Config{Runner: r})
	assert.Contains(t, r.capturedTools, "Bash")
}

func TestRunIncludesServiceInPrompt(t *testing.T) {
	r := &stubRunner{response: "ok"}
	_, _ = diagnose.Run(context.Background(), diagnose.Config{
		Service: "myservice",
		Runner:  r,
	})
	assert.Contains(t, r.capturedPrompt, "myservice")
}

func TestRunIncludesLogCmdInPrompt(t *testing.T) {
	r := &stubRunner{response: "ok"}
	_, _ = diagnose.Run(context.Background(), diagnose.Config{
		LogCmd: "cat /var/log/myapp.log",
		Runner: r,
	})
	assert.Contains(t, r.capturedPrompt, "cat /var/log/myapp.log")
}

func TestRunDefaultLogCmdInPrompt(t *testing.T) {
	r := &stubRunner{response: "ok"}
	_, _ = diagnose.Run(context.Background(), diagnose.Config{Runner: r})
	assert.Contains(t, r.capturedPrompt, diagnose.DefaultLogCmd())
}

func TestRunPromptContainsReportSections(t *testing.T) {
	r := &stubRunner{response: "ok"}
	_, _ = diagnose.Run(context.Background(), diagnose.Config{Runner: r})
	for _, section := range []string{"Root cause", "Evidence", "Fix", "Confidence"} {
		assert.Contains(t, r.capturedPrompt, section, "prompt missing section: %s", section)
	}
}

func TestDefaultLogCmd(t *testing.T) {
	assert.NotEmpty(t, diagnose.DefaultLogCmd())
}

func TestRunNoServiceSkipsTargetedGrep(t *testing.T) {
	r := &stubRunner{response: "ok"}
	_, _ = diagnose.Run(context.Background(), diagnose.Config{Runner: r})
	// When Service is empty, prompt should not contain grep -i ""
	assert.NotContains(t, r.capturedPrompt, `grep -i ""`)
}

func TestRunErrorPropagates(t *testing.T) {
	r := diagnose.RunnerFunc(func(_ context.Context, _ string, _ []string) (string, error) {
		return "", fmt.Errorf("simulated failure")
	})
	_, err := diagnose.Run(context.Background(), diagnose.Config{Runner: r})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated failure")
}

func TestRunNilRunnerReturnsError(t *testing.T) {
	_, err := diagnose.Run(context.Background(), diagnose.Config{})
	require.Error(t, err)
}

func TestRunRequestsAllTools(t *testing.T) {
	r := &stubRunner{response: "ok"}
	_, _ = diagnose.Run(context.Background(), diagnose.Config{Runner: r})
	assert.Contains(t, r.capturedTools, "Bash")
	assert.Contains(t, r.capturedTools, "Read")
	assert.Contains(t, r.capturedTools, "Glob")
}
