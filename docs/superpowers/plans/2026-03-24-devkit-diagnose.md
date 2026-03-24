# devkit diagnose Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `devkit diagnose` — a local failure diagnosis command that runs an LLM agent against project logs and system state to report root cause, evidence, fix, and confidence.

**Architecture:** Follows the existing hexagonal pattern: a new `internal/diagnose` package defines a `Runner` port and `Run` function; `cmd/devkit/main.go` wires up a cobra subcommand. A new `BashTool` is added to `internal/tools` so the agent can execute shell commands (journalctl, mount, ps, etc.) — it is available by name to any runner but only requested by diagnose.

**Tech Stack:** Go, `github.com/anthropics/anthropic-sdk-go`, `github.com/spf13/cobra`, `github.com/stretchr/testify`, existing `internal/tools`, `internal/loop`, `internal/log`.

---

## File Map

| Action | Path | Purpose |
|--------|------|---------|
| Modify | `internal/tools/tools.go` | Add `BashTool(maxBytes int) Tool` |
| Modify | `internal/tools/tools_test.go` | Tests for BashTool |
| Create | `internal/diagnose/diagnose.go` | `Runner` interface, `Config`, `Run` |
| Create | `internal/diagnose/diagnose_test.go` | Unit tests with stub runner |
| Modify | `cmd/devkit/config.go` | Add `Diagnose struct` to `Config` |
| Modify | `cmd/devkit/runner.go` | Add BashTool to `agentRunner.allTools` |
| Modify | `cmd/devkit/main.go` | Add `diagnose` cobra subcommand |
| Modify | `install.sh` | Add `diagnose = true` to generated components block |

---

## Task 1: BashTool

**Files:**
- Modify: `internal/tools/tools.go`
- Modify: `internal/tools/tools_test.go`

- [ ] **Step 1.1: Write failing tests for BashTool**

Add to `internal/tools/tools_test.go`:

```go
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
    // Non-zero exit returns output as error string, not a Go error
    require.NoError(t, err)
    assert.Contains(t, result, "exit status 1")
}
```

- [ ] **Step 1.2: Verify tests fail**

```bash
cd ~/dev/devkit && go test ./internal/tools/ -run TestBashTool -v
```

Expected: `FAIL` — `BashTool` undefined.

- [ ] **Step 1.3: Implement BashTool**

Add to `internal/tools/tools.go` after `GrepTool`:

```go
// BashTool returns a Tool that executes a shell command via "sh -c" and returns
// combined stdout+stderr. Output is capped at maxBytes; a "[truncated]" suffix
// is appended when the cap is reached. Non-zero exit codes are reported in the
// output (not as a Go error) so the agent sees the failure and can react.
// Note: the cap is applied to the raw output before appending the exit suffix,
// so the final string may exceed maxBytes by the length of the "(exit: ...)" suffix.
// This is intentional — the exit status is always visible.
func BashTool(maxBytes int) Tool {
	return Tool{
		Definition: anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        "Bash",
			Description: anthropic.String("Execute a shell command and return combined stdout+stderr."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]interface{}{
					"command": map[string]string{
						"type":        "string",
						"description": "Shell command to execute",
					},
					// required is not set here; consistent with other tools in this package.
				},
			},
		}},
		Handler: HandlerFunc(func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", err
			}
			cmd := exec.CommandContext(ctx, "sh", "-c", args.Command)
			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			runErr := cmd.Run()
			out := buf.String()
			if len(out) > maxBytes {
				out = out[:maxBytes] + "[truncated]"
			}
			if runErr != nil && buf.Len() == 0 {
				out = runErr.Error()
			} else if runErr != nil {
				out = fmt.Sprintf("%s\n(exit: %s)", out, runErr.Error())
			}
			return out, nil
		}),
	}
}
```

Add missing imports to `tools.go`:
- `"bytes"`
- `"os/exec"`

- [ ] **Step 1.4: Verify tests pass**

```bash
cd ~/dev/devkit && go test ./internal/tools/ -v
```

Expected: all tests PASS including the four new BashTool tests.

- [ ] **Step 1.5: Verify build is clean**

```bash
cd ~/dev/devkit && go build ./...
```

Expected: no output (success).

- [ ] **Step 1.6: Commit**

```bash
cd ~/dev/devkit && git add internal/tools/
git commit -m "feat(tools): add BashTool with output cap and stderr capture"
```

---

## Task 2: internal/diagnose package

**Files:**
- Create: `internal/diagnose/diagnose.go`
- Create: `internal/diagnose/diagnose_test.go`

- [ ] **Step 2.1: Write failing tests**

Create `internal/diagnose/diagnose_test.go`:

```go
// internal/diagnose/diagnose_test.go
package diagnose_test

import (
	"context"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/diagnose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRunner struct {
	response      string
	capturedTools []string
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
	assert.Contains(t, r.capturedPrompt, "journalctl")
}

func TestRunPromptContainsReportSections(t *testing.T) {
	r := &stubRunner{response: "ok"}
	_, _ = diagnose.Run(context.Background(), diagnose.Config{Runner: r})
	for _, section := range []string{"Root cause", "Evidence", "Fix", "Confidence"} {
		assert.True(t, strings.Contains(r.capturedPrompt, section),
			"prompt missing section: %s", section)
	}
}

func TestDefaultLogCmd(t *testing.T) {
	assert.Contains(t, diagnose.DefaultLogCmd(), "journalctl")
}

func TestRunNoServiceSkipsTargetedGrep(t *testing.T) {
	r := &stubRunner{response: "ok"}
	_, _ = diagnose.Run(context.Background(), diagnose.Config{Runner: r})
	// When Service is empty, prompt should not contain grep -i ""
	assert.NotContains(t, r.capturedPrompt, `grep -i ""`)
}
```

- [ ] **Step 2.2: Verify tests fail**

```bash
cd ~/dev/devkit && go test ./internal/diagnose/ -v 2>&1 | head -20
```

Expected: `FAIL` — package not found.

- [ ] **Step 2.3: Implement diagnose package**

Create `internal/diagnose/diagnose.go` with exactly this content:

```go
// internal/diagnose/diagnose.go
package diagnose

import (
	"context"
	"fmt"
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

const defaultLogCmd = "journalctl -n 200 --no-pager"

// DefaultLogCmd returns the log command used when Config.LogCmd is empty.
func DefaultLogCmd() string {
	return defaultLogCmd
}

// Run executes a diagnosis agent and returns its report.
func Run(ctx context.Context, cfg Config) (string, error) {
	logCmd := cfg.LogCmd
	if logCmd == "" {
		logCmd = defaultLogCmd
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
```

- [ ] **Step 2.4: Verify tests pass**

```bash
cd ~/dev/devkit && go test ./internal/diagnose/ -v
```

Expected: all 6 tests PASS.

- [ ] **Step 2.5: Commit**

```bash
cd ~/dev/devkit && git add internal/diagnose/
git commit -m "feat(diagnose): add internal/diagnose package with Runner port and prompt builder"
```

---

## Task 3: Wire diagnose into cmd/devkit

**Files:**
- Modify: `cmd/devkit/config.go` — add Diagnose config section
- Modify: `cmd/devkit/runner.go` — add BashTool to agentRunner
- Modify: `cmd/devkit/main.go` — add diagnose subcommand

### Task 3a: Config

- [ ] **Step 3a.1: Add Diagnose section to Config struct**

In `cmd/devkit/config.go`, add to the `Config` struct after the `Components` block:

```go
Diagnose struct {
    LogCmd  string `toml:"log_cmd"`
    Service string `toml:"service"`
} `toml:"diagnose"`
```

Also add `Diagnose bool` to the `Components` struct:

```go
Components struct {
    Council bool `toml:"council"`
    Review  bool `toml:"review"`
    Meta    bool `toml:"meta"`
    CIAgent bool `toml:"ci_agent"`
    Diagnose bool `toml:"diagnose"`
} `toml:"components"`
```

- [ ] **Step 3a.2: Verify build**

```bash
cd ~/dev/devkit && go build ./cmd/devkit/
```

Expected: no output (success).

### Task 3b: agentRunner adds BashTool

- [ ] **Step 3b.1: Add BashTool to agentRunner.Run**

In `cmd/devkit/runner.go`, in the `Run` method, update `allTools` and add a comment documenting the name-resolution assumption:

```go
// allTools lists every tool available to any runner. Tools are filtered by
// name (via t.Definition.OfTool.Name) when a caller provides toolNames — so
// all Tool values here must use the OfTool variant, not OfToolBash or other
// ToolUnionParam alternatives, or they will silently not match.
allTools := []tools.Tool{
    tools.ReadTool(wd),
    tools.GlobTool(wd),
    tools.GrepTool(wd),
    tools.BashTool(30_000),
}
```

BashTool is only activated when requested by name — existing commands (council, review, meta) don't request "Bash", so their behavior is unchanged.

- [ ] **Step 3b.2: Verify build**

```bash
cd ~/dev/devkit && go build ./cmd/devkit/
```

Expected: no output (success).

### Task 3c: diagnose subcommand

- [ ] **Step 3c.1: Add diagnose subcommand to main.go**

In `cmd/devkit/main.go`, add imports at the top:
```go
"github.com/89jobrien/devkit/internal/diagnose"
```

Add the diagnose subcommand before `root.AddCommand(...)`:

```go
// diagnose subcommand
var diagnoseService, diagnoseLogCmd string
diagnoseCmd := &cobra.Command{
    Use:   "diagnose",
    Short: "Diagnose a service failure from logs and system state",
    RunE: func(cmd *cobra.Command, args []string) error {
        cfg, err := LoadConfig()
        if err != nil {
            return err
        }
        if cfg.Project.Name != "" {
            os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
        }

        // Flags override config; config overrides defaults.
        service := diagnoseService
        if service == "" {
            service = cfg.Diagnose.Service
        }
        logCmd := diagnoseLogCmd
        if logCmd == "" {
            logCmd = cfg.Diagnose.LogCmd
        }

        runner := newAgentRunner()
        sha := devlog.GitShortSHA()
        id := devlog.Start("diagnose", map[string]string{"service": service})
        start := time.Now()

        result, err := diagnose.Run(cmd.Context(), diagnose.Config{
            Service: service,
            LogCmd:  logCmd,
            Runner: diagnose.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
                return runner.Run(ctx, prompt, ts)
            }),
        })
        if err != nil {
            return err
        }

        fmt.Println(result)
        devlog.Complete(id, "diagnose", map[string]string{"service": service}, result, time.Since(start))
        path, _ := devlog.SaveCommitLog(sha, "diagnose", result, map[string]string{"service": service})
        fmt.Printf("\nLogged to: %s\n", path)
        return nil
    },
}
diagnoseCmd.Flags().StringVar(&diagnoseService, "service", "", "Service/component to focus on")
diagnoseCmd.Flags().StringVar(&diagnoseLogCmd, "log-cmd", "", "Shell command to fetch logs (overrides .devkit.toml)")
```

Update the `root.AddCommand` line:

```go
root.AddCommand(councilCmd, reviewCmd, metaCmd, diagnoseCmd)
```

- [ ] **Step 3c.2: Verify build**

```bash
cd ~/dev/devkit && go build ./cmd/devkit/ && ./devkit diagnose --help
```

Expected:
```
Diagnose a service failure from logs and system state

Usage:
  devkit diagnose [flags]

Flags:
      --log-cmd string   Shell command to fetch logs (overrides .devkit.toml)
  -h, --help             help for diagnose
      --service string   Service/component to focus on
```

- [ ] **Step 3c.3: Run full test suite**

```bash
cd ~/dev/devkit && go test ./...
```

Expected: all tests PASS (count should increase from 21 to 27 with new diagnose + BashTool tests).

- [ ] **Step 3c.4: Build both binaries**

```bash
cd ~/dev/devkit && go build ./cmd/devkit ./cmd/ci-agent
```

Expected: no output (success).

- [ ] **Step 3c.5: Commit**

```bash
cd ~/dev/devkit && git add cmd/devkit/ && git commit -m "feat: add devkit diagnose subcommand with BashTool support"
```

---

## Task 4: Update install.sh

**Files:**
- Modify: `install.sh`

`install.sh` uses a two-pass component system: a `gum choose` selection, a `case` block (in a subshell), and a `grep` re-derivation (because the subshell doesn't propagate variables). All four locations must be updated together, plus the TOML heredoc.

- [ ] **Step 4.1: Add `diagnose` to the `gum choose` line (line 26)**

Change:
```bash
components="$(gum choose --no-limit --selected council,review,meta,ci_agent council review meta ci_agent)"
```
To:
```bash
components="$(gum choose --no-limit --selected council,review,meta,ci_agent,diagnose council review meta ci_agent diagnose)"
```

- [ ] **Step 4.2: Add `has_diagnose=false` to the boolean init block (after line 35)**

After `has_ci_agent=false`, add:
```bash
has_diagnose=false
```

- [ ] **Step 4.3: Add `diagnose` arm to the `case` block (lines 37–42)**

Add before the `esac`:
```bash
    diagnose)  has_diagnose=true ;;
```

- [ ] **Step 4.4: Add grep re-derivation for diagnose (after line 49)**

After the `has_ci_agent` grep line, add:
```bash
if echo "$components" | grep -q "diagnose"; then has_diagnose=true; fi
```

- [ ] **Step 4.5: Add `diagnose` to the TOML `[components]` block and add `[diagnose]` section**

In the `cat > .devkit.toml` heredoc, update `[components]`:
```toml
[components]
council  = $has_council
review   = $has_review
meta     = $has_meta
ci_agent = $has_ci_agent
diagnose = $has_diagnose
```

Add after the `[council]` block:
```toml
[diagnose]
# log_cmd = "journalctl -n 200 --no-pager"   # uncomment and customize if needed
# service = ""                                 # focus on a specific service
```

- [ ] **Step 4.6: Verify install.sh is valid bash**

```bash
bash -n ~/dev/devkit/install.sh
```

Expected: no output (no syntax errors).

- [ ] **Step 4.7: Commit**

```bash
cd ~/dev/devkit && git add install.sh && git commit -m "feat(install): add diagnose component to gum choose, case, re-derivation, and TOML"
```

---

## Task 5: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 5.1: Add diagnose to CLAUDE.md development section**

In `CLAUDE.md`, under the Development section (after the existing go test/build lines), add:

```markdown
- `devkit diagnose [--service <name>] [--log-cmd <cmd>]` — run LLM diagnosis on local service logs
```

- [ ] **Step 5.2: Commit**

```bash
cd ~/dev/devkit && git add CLAUDE.md && git commit -m "docs: document devkit diagnose in CLAUDE.md"
```

---

## Verification

After all tasks, run the full suite and smoke-test:

```bash
# All tests green
cd ~/dev/devkit && go test ./... -v 2>&1 | tail -20

# Binary compiles and exposes correct commands
./devkit --help | grep diagnose

# Flags work
./devkit diagnose --help
```

Expected test output:
```
ok  	github.com/89jobrien/devkit/internal/council	0.XXXs
ok  	github.com/89jobrien/devkit/internal/diagnose	0.XXXs
ok  	github.com/89jobrien/devkit/internal/log	0.XXXs
ok  	github.com/89jobrien/devkit/internal/loop	0.XXXs
ok  	github.com/89jobrien/devkit/internal/meta	0.XXXs
ok  	github.com/89jobrien/devkit/internal/platform	0.XXXs
ok  	github.com/89jobrien/devkit/internal/review	0.XXXs
ok  	github.com/89jobrien/devkit/internal/tools	0.XXXs
```

Expected `./devkit --help`:
```
Available Commands:
  council    Multi-role branch analysis
  diagnose   Diagnose a service failure from logs and system state
  meta       Design and run parallel agents for any task
  review     AI diff review
```
