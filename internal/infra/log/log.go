// internal/log/log.go
package log

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunID is an ISO-8601 timestamp used to correlate start/complete entries.
type RunID string

// ProjectName returns the project name for log namespacing.
// Priority: DEVKIT_PROJECT env → git repo basename → "unknown".
func ProjectName() string {
	if v := os.Getenv("DEVKIT_PROJECT"); v != "" {
		return v
	}
	// Try git basename
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		return filepath.Base(strings.TrimSpace(string(out)))
	}
	return "unknown"
}

// GitShortSHA returns the short git SHA of HEAD, or "unknown".
func GitShortSHA() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func logDir() string {
	if v := os.Getenv("DEVKIT_LOG_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".dev-agents")
}

func projectDir() string {
	return filepath.Join(logDir(), ProjectName())
}

func jsonlPath() string {
	return filepath.Join(projectDir(), "agent-runs.jsonl")
}

func appendJSONL(record map[string]any) {
	path := jsonlPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(record)
	_, _ = f.Write(append(b, '\n'))
}

// Start records a run-start entry and returns a RunID.
func Start(command string, args map[string]string) RunID {
	id := RunID(time.Now().UTC().Format(time.RFC3339Nano))
	appendJSONL(map[string]any{
		"run_id":  string(id),
		"command": command,
		"args":    args,
		"status":  "running",
	})
	return id
}

// Complete records a run-completion entry.
func Complete(id RunID, command string, args map[string]string, output string, duration time.Duration) {
	appendJSONL(map[string]any{
		"run_id":      string(id),
		"command":     command,
		"args":        args,
		"status":      "complete",
		"duration_ms": duration.Milliseconds(),
		"output":      output,
	})
}

// SaveCommitLog writes ~/.dev-agents/<project>/ai-logs/<sha>-<command>.md and returns the path.
func SaveCommitLog(sha, command, content string, meta map[string]string) (string, error) {
	dir := filepath.Join(projectDir(), "ai-logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().Unix()
	path := filepath.Join(dir, fmt.Sprintf("%s-%d-%s-%s.md", ProjectName(), ts, sha, command))

	var header strings.Builder
	header.WriteString(fmt.Sprintf("# %s · %s\n\n", command, sha))
	for k, v := range meta {
		header.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
	}
	header.WriteString("\n---\n\n")

	if err := os.WriteFile(path, []byte(header.String()+content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
