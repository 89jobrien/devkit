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

// GatherRepoContext returns a markdown snapshot of the current repo state:
// CLAUDE.md/AGENTS.md/README.md previews, recent commits, working tree, and file structure.
func GatherRepoContext() string {
	run := func(args ...string) string {
		out, _ := exec.Command(args[0], args[1:]...).Output()
		return string(out)
	}
	var sb strings.Builder
	for _, f := range []string{"CLAUDE.md", "AGENTS.md", "README.md"} {
		if data, err := os.ReadFile(f); err == nil {
			preview := data
			if len(preview) > 2000 {
				preview = preview[:2000]
			}
			sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", f, string(preview)))
		}
	}
	sb.WriteString("## Recent commits\n" + run("git", "log", "--oneline", "-20"))
	sb.WriteString("\n## Working tree\n" + run("git", "status", "--short"))
	allFiles := run("git", "ls-files")
	lines := strings.Split(strings.TrimSpace(allFiles), "\n")
	if len(lines) > 150 {
		lines = lines[:150]
	}
	sb.WriteString("\n## Structure (first 150 paths)\n" + strings.Join(lines, "\n"))
	return sb.String()
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
	ts := time.Now().Format("20060102")
	path := filepath.Join(dir, fmt.Sprintf("%s-%s-%s-%s.md", ProjectName(), ts, sha, command))

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
