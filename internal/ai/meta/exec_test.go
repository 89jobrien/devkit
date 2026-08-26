package meta_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/ai/meta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func execRunner() meta.RunnerFunc {
	return func(_ context.Context, prompt string, _ []string) (string, error) {
		switch {
		case strings.Contains(prompt, "meta-agent designer"):
			return `[{"name":"checker","role":"check code","prompt":"inspect code","tools":["Read"]}]`, nil
		case strings.Contains(prompt, "Synthesize outputs"):
			return "summary output", nil
		default:
			return "worker output", nil
		}
	}
}

func TestExecWritesWorkerAndSummaryOutput(t *testing.T) {
	t.Setenv("DEVKIT_LOG_DIR", t.TempDir())
	t.Setenv("DEVKIT_PROJECT", "testproj")
	var output bytes.Buffer

	result, err := meta.Exec(context.Background(), "audit", "repo", "docs", execRunner(), &output, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Contains(t, output.String(), "---- checker ----\nworker output")
	assert.Contains(t, output.String(), "---- SYNTHESIS ----\nsummary output")
}

func TestExecCreatesCommitLog(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DEVKIT_LOG_DIR", dir)
	t.Setenv("DEVKIT_PROJECT", "testproj")

	result, err := meta.Exec(context.Background(), "audit", "repo", "docs", execRunner(), &bytes.Buffer{}, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.LogPath)
	data, err := os.ReadFile(result.LogPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "## checker\nworker output")
	assert.Contains(t, string(data), "## Synthesis\nsummary output")
}

func TestExecNoSynthesisOmitsSummaryOutput(t *testing.T) {
	t.Setenv("DEVKIT_LOG_DIR", t.TempDir())
	t.Setenv("DEVKIT_PROJECT", "testproj")
	var output bytes.Buffer

	result, err := meta.Exec(context.Background(), "audit", "repo", "docs", execRunner(), &output, true)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Contains(t, output.String(), "worker output")
	assert.NotContains(t, output.String(), "SYNTHESIS")
	data, err := os.ReadFile(result.LogPath)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "## Synthesis")
}

func TestExecReturnsRunnerErrors(t *testing.T) {
	wantErr := errors.New("runner failed")
	runner := meta.RunnerFunc(func(context.Context, string, []string) (string, error) {
		return "", wantErr
	})

	result, err := meta.Exec(context.Background(), "audit", "repo", "docs", runner, &bytes.Buffer{}, false)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, wantErr)
}
