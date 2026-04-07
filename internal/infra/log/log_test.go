// internal/log/log_test.go
package log_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/89jobrien/devkit/internal/infra/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartComplete(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DEVKIT_LOG_DIR", dir)
	t.Setenv("DEVKIT_PROJECT", "testproj")

	id := log.Start("council", map[string]string{"base": "main"})
	assert.NotEmpty(t, id)

	log.Complete(id, "council", map[string]string{"base": "main"}, "output text", 1500*time.Millisecond)

	data, err := os.ReadFile(filepath.Join(dir, "testproj", "agent-runs.jsonl"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Len(t, lines, 2)

	var start map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &start))
	assert.Equal(t, "council", start["command"])
	assert.Equal(t, "running", start["status"])

	var complete map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &complete))
	assert.Equal(t, "complete", complete["status"])
	assert.Equal(t, "output text", complete["output"])
}

func TestSaveCommitLog(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DEVKIT_LOG_DIR", dir)
	t.Setenv("DEVKIT_PROJECT", "testproj")

	path, err := log.SaveCommitLog("abc1234", "council", "## Results\nfoo", map[string]string{"mode": "core"})
	require.NoError(t, err)
	assert.Contains(t, path, "abc1234-council.md")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "## Results")
	assert.Contains(t, string(data), "mode: core")
}

func TestProjectNameFallback(t *testing.T) {
	t.Setenv("DEVKIT_PROJECT", "")
	// Should not panic; falls back to git or "unknown"
	name := log.ProjectName()
	assert.NotEmpty(t, name)
}

