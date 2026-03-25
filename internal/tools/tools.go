// internal/tools/tools.go
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/anthropics/anthropic-sdk-go"
)

// Handler is the port for executing a tool call.
// The input is raw JSON from the Anthropic tool_use block.
type Handler interface {
	Handle(ctx context.Context, input json.RawMessage) (string, error)
}

// HandlerFunc is a function adapter for Handler.
type HandlerFunc func(ctx context.Context, input json.RawMessage) (string, error)

func (f HandlerFunc) Handle(ctx context.Context, input json.RawMessage) (string, error) {
	return f(ctx, input)
}

// Tool pairs an Anthropic tool definition with its handler.
type Tool struct {
	Definition anthropic.ToolUnionParam
	Handler    Handler
}

func validatePath(root, rel string) (string, error) {
	abs := filepath.Join(root, rel)
	clean, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	rootClean, _ := filepath.Abs(root)
	if !strings.HasPrefix(clean, rootClean+string(os.PathSeparator)) && clean != rootClean {
		return "", fmt.Errorf("path %q is outside working directory", rel)
	}
	return clean, nil
}

// ReadTool returns a Tool that reads a file relative to root.
func ReadTool(root string) Tool {
	return Tool{
		Definition: anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        "Read",
			Description: anthropic.String("Read a file and return its contents."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]interface{}{
					"path": map[string]string{"type": "string", "description": "File path relative to working directory"},
				},
			},
		}},
		Handler: HandlerFunc(func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", err
			}
			abs, err := validatePath(root, args.Path)
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				return "", err
			}
			return string(data), nil
		}),
	}
}

// GlobTool returns a Tool that matches a glob pattern relative to root using fd.
// Supports recursive patterns (e.g. "**/*.go").
func GlobTool(root string) Tool {
	return Tool{
		Definition: anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        "Glob",
			Description: anthropic.String("Match files against a glob pattern. Supports recursive patterns (e.g. '**/*.go'). Returns newline-separated paths."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]interface{}{
					"pattern": map[string]string{"type": "string", "description": "Glob pattern (e.g. '*.go', '**/*.go')"},
				},
			},
		}},
		Handler: HandlerFunc(func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Pattern string `json:"pattern"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", err
			}
			if args.Pattern == "" {
				return "", fmt.Errorf("pattern is required")
			}
			if _, err := exec.LookPath("fd"); err != nil {
				return "", fmt.Errorf("GlobTool requires 'fd' (https://github.com/sharkdp/fd): not found in PATH")
			}
			cmd := exec.CommandContext(ctx, "fd", "--glob", "--hidden", "--no-ignore", args.Pattern)
			cmd.Dir = root
			out, err := cmd.Output()
			if err != nil {
				return "", fmt.Errorf("fd: %w", err)
			}
			return strings.TrimRight(string(out), "\n"), nil
		}),
	}
}

// GrepTool returns a Tool that searches file content for a regex pattern using rg (ripgrep).
// Glob patterns support recursion (e.g. "**/*.go"). Returns file:line matches.
func GrepTool(root string) Tool {
	return Tool{
		Definition: anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        "Grep",
			Description: anthropic.String("Search file content for a regex pattern. Supports recursive glob patterns (e.g. '**/*.go'). Returns file:line matches."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]interface{}{
					"pattern": map[string]string{"type": "string", "description": "Regular expression pattern"},
					"glob":    map[string]string{"type": "string", "description": "Glob pattern to filter files (e.g. '*.go', '**/*.go')"},
				},
			},
		}},
		Handler: HandlerFunc(func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Pattern string `json:"pattern"`
				Glob    string `json:"glob"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", err
			}
			if args.Pattern == "" {
				return "", fmt.Errorf("pattern is required")
			}
			if _, err := exec.LookPath("rg"); err != nil {
				return "", fmt.Errorf("GrepTool requires 'rg' (https://github.com/BurntSushi/ripgrep): not found in PATH")
			}
			argv := []string{"--with-filename", "--line-number", "--no-heading", "--hidden", "--no-ignore", args.Pattern}
			if args.Glob != "" {
				argv = append(argv, "--glob", args.Glob)
			}
			cmd := exec.CommandContext(ctx, "rg", argv...)
			cmd.Dir = root
			out, err := cmd.Output()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
					return "", nil // no matches
				}
				return "", fmt.Errorf("rg: %w", err)
			}
			return strings.TrimRight(string(out), "\n"), nil
		}),
	}
}

// BashTool returns a Tool that executes a shell command via "sh -c" and returns
// combined stdout+stderr. Output is capped at maxBytes; a "[truncated]" suffix
// is appended when the cap is reached. Non-zero exit codes are reported in the
// output (not as a Go error) so the agent sees the failure and can react.
// Note: the cap is applied to raw output before appending the exit suffix,
// so the final string may exceed maxBytes by the length of "(exit: ...)".
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
			if args.Command == "" {
				return "", fmt.Errorf("command is required")
			}
			cmd := exec.Command("sh", "-c", args.Command)
			// Place the command in its own process group so that context
			// cancellation can kill the entire group (sh + its children).
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			if err := cmd.Start(); err != nil {
				return err.Error(), nil
			}
			done := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					// Kill the entire process group.
					if cmd.Process != nil {
						_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					}
				case <-done:
				}
			}()
			runErr := cmd.Wait()
			close(done)
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

// Definitions extracts the Anthropic tool definitions from a slice of Tools.
func Definitions(ts []Tool) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, len(ts))
	for i, t := range ts {
		out[i] = t.Definition
	}
	return out
}
