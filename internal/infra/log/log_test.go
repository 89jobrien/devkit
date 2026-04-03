// internal/log/log_test.go
package log_test

import (
	"encoding/json"
	"os"
	"os/exec"
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

// initGitRepo creates a minimal git repo in dir with one commit.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@t.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@t.com",
			"GIT_CONFIG_NOSYSTEM=1", "HOME="+dir,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("git", "init", "-b", "main")
	run("git", "config", "commit.gpgsign", "false")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test Repo\nHello world.\n"), 0o644)
	run("git", "add", ".")
	run("git", "commit", "-m", "initial commit")
}

func TestGatherRepoContextSections(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(orig) })

	result := log.GatherRepoContext()

	assert.Contains(t, result, "## Recent commits")
	assert.Contains(t, result, "## Working tree")
	assert.Contains(t, result, "## Structure")
	// README.md preview should be included
	assert.Contains(t, result, "### README.md")
	assert.Contains(t, result, "Hello world.")
}

func TestGatherRepoContextTruncatesLargeReadme(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Overwrite README.md with >2000 bytes
	big := strings.Repeat("x", 3000)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte(big), 0o644))

	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(orig) })

	result := log.GatherRepoContext()

	// Should appear but truncated to 2000 bytes, not 3000
	assert.Contains(t, result, "### README.md")
	assert.NotContains(t, result, strings.Repeat("x", 2001))
}

func TestGatherRepoContextNoGit(t *testing.T) {
	dir := t.TempDir()
	// No git repo — git commands will fail; should not panic

	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(orig) })

	// Should return something (possibly empty sections) without panicking
	result := log.GatherRepoContext()
	assert.Contains(t, result, "## Recent commits")
	assert.Contains(t, result, "## Working tree")
}
