package repometa_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/repometa"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initGitRepo creates a minimal git repo in dir with one commit.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=noreply",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=noreply",
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

func TestGatherSnapshotSections(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(orig) })

	result := repometa.GatherSnapshot()

	assert.Contains(t, result, "## Recent commits")
	assert.Contains(t, result, "## Working tree")
	assert.Contains(t, result, "## Structure")
	assert.Contains(t, result, "### README.md")
	assert.Contains(t, result, "Hello world.")
}

func TestGatherSnapshotTruncatesLargeReadme(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	big := strings.Repeat("x", 3000)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte(big), 0o644))

	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(orig) })

	result := repometa.GatherSnapshot()

	assert.Contains(t, result, "### README.md")
	assert.NotContains(t, result, strings.Repeat("x", 2001))
}

func TestGatherSnapshotNoGit(t *testing.T) {
	dir := t.TempDir()

	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(orig) })

	result := repometa.GatherSnapshot()
	assert.Contains(t, result, "## Recent commits")
	assert.Contains(t, result, "## Working tree")
}
