// internal/tools/tools_test.go
package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/89jobrien/devkit/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadTool(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0o644))

	tool := tools.ReadTool(dir)
	input, _ := json.Marshal(map[string]string{"path": "hello.txt"})
	result, err := tool.Handler.Handle(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "hello world", result)
}

func TestReadToolRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	tool := tools.ReadTool(dir)
	input, _ := json.Marshal(map[string]string{"path": "../secret"})
	_, err := tool.Handler.Handle(context.Background(), input)
	assert.ErrorContains(t, err, "outside")
}

func TestGlobTool(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte(""), 0o644))

	tool := tools.GlobTool(dir)
	input, _ := json.Marshal(map[string]string{"pattern": "*.go"})
	result, err := tool.Handler.Handle(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result, "a.go")
	assert.Contains(t, result, "b.go")
	assert.NotContains(t, result, "c.txt")
}

func TestGrepTool(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644))

	tool := tools.GrepTool(dir)
	input, _ := json.Marshal(map[string]string{"pattern": "func main", "glob": "*.go"})
	result, err := tool.Handler.Handle(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result, "main.go:2")
	assert.Contains(t, result, "func main")
}

func TestBashToolRunsCommand(t *testing.T) {
	tool := tools.BashTool(4096)
	input, _ := json.Marshal(map[string]string{"command": "echo hello"})
	result, err := tool.Handler.Handle(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", result)
}

func TestBashToolCapsOutput(t *testing.T) {
	tool := tools.BashTool(10)
	input, _ := json.Marshal(map[string]string{"command": "printf '%0.s1234567890' {1..100}"})
	result, err := tool.Handler.Handle(context.Background(), input)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result), 10+len("[truncated]"))
}

func TestBashToolCapturesStderr(t *testing.T) {
	tool := tools.BashTool(4096)
	input, _ := json.Marshal(map[string]string{"command": "echo err >&2"})
	result, err := tool.Handler.Handle(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result, "err")
}

func TestBashToolNonZeroExit(t *testing.T) {
	tool := tools.BashTool(4096)
	input, _ := json.Marshal(map[string]string{"command": "exit 1"})
	result, err := tool.Handler.Handle(context.Background(), input)
	// Non-zero exit surfaces in output, not as a Go error
	require.NoError(t, err)
	assert.Contains(t, result, "exit status 1")
}
