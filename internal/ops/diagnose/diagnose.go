// internal/diagnose/diagnose.go
package diagnose

import (
	"context"
	"fmt"
	"runtime"
)

// Runner is the port for executing LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds parameters for a diagnosis run.
type Config struct {
	// Service is an optional service/component name to focus on.
	// When empty, the agent focuses on the most recent failure.
	Service string
	// LogCmd is the shell command used to fetch logs.
	// Defaults to "journalctl -n 200 --no-pager" when empty.
	LogCmd string
	Runner Runner
}

// DefaultLogCmd returns the log command used when Config.LogCmd is empty.
// The command is chosen based on the current OS.
func DefaultLogCmd() string {
	switch runtime.GOOS {
	case "darwin":
		return "log show --last 5m --style syslog"
	default:
		return "journalctl -n 200 --no-pager"
	}
}

// Run executes a diagnosis agent and returns its report.
func Run(ctx context.Context, cfg Config) (string, error) {
	if cfg.Runner == nil {
		return "", fmt.Errorf("diagnose: Runner is required")
	}

	logCmd := cfg.LogCmd
	if logCmd == "" {
		logCmd = DefaultLogCmd()
	}

	focus := "Focus on the most recent failure."
	if cfg.Service != "" {
		focus = fmt.Sprintf("Focus on service/component: %s", cfg.Service)
	}

	// When Service is empty, grep -i "" matches everything — skip to ps head directly.
	var psStep string
	if cfg.Service != "" {
		psStep = fmt.Sprintf(`ps aux | grep -v grep | grep -i "%s" 2>/dev/null || ps aux | head -20`, cfg.Service)
	} else {
		psStep = "ps aux | head -20"
	}

	prompt := fmt.Sprintf(`Diagnose a service failure. %s

Gather evidence in this order:
1. Run: %s
   (If that fails, try alternative log sources like /var/log/ or journalctl with different flags)
2. Run: mount | grep -v tmpfs | grep -v devtmpfs
3. Run: %s
4. Check for common failure patterns:
   - Out of memory / OOM killer (grep for "killed process" or "oom" in logs)
   - Permission denied / EACCES (file or socket permissions)
   - Port already in use / EADDRINUSE
   - Disk full / ENOSPC
   - Configuration parse errors (look for "error", "fatal", "panic" near startup)
   - Dependency not ready (database, upstream service connection refused)

Report exactly these sections:
- **Root cause**: the specific error and why it occurred
- **Evidence**: exact log lines or command output that confirms it
- **Fix**: minimal change needed (config, command, or code pointer)
- **Confidence**: high / medium / low`,
		focus, logCmd, psStep)

	tools := []string{"Bash", "Read", "Glob"}
	return cfg.Runner.Run(ctx, prompt, tools)
}
