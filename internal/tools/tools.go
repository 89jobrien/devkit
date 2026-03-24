// internal/tools/tools.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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

// GlobTool returns a Tool that matches a glob pattern relative to root.
func GlobTool(root string) Tool {
	return Tool{
		Definition: anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        "Glob",
			Description: anthropic.String("Match files against a glob pattern. Returns newline-separated paths."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]interface{}{
					"pattern": map[string]string{"type": "string", "description": "Glob pattern"},
				},
			},
		}},
		Handler: HandlerFunc(func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Pattern string `json:"pattern"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", err
			}
			matches, err := filepath.Glob(filepath.Join(root, args.Pattern))
			if err != nil {
				return "", err
			}
			rel := make([]string, 0, len(matches))
			for _, m := range matches {
				r, _ := filepath.Rel(root, m)
				rel = append(rel, r)
			}
			return strings.Join(rel, "\n"), nil
		}),
	}
}

// GrepTool returns a Tool that searches file content for a regex pattern.
func GrepTool(root string) Tool {
	return Tool{
		Definition: anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        "Grep",
			Description: anthropic.String("Search file content for a regex pattern. Returns file:line matches."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]interface{}{
					"pattern": map[string]string{"type": "string", "description": "Regular expression pattern"},
					"glob":    map[string]string{"type": "string", "description": "Glob pattern to filter files (e.g. '*.go')"},
				},
			},
		}},
		Handler: HandlerFunc(func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Pattern string `json:"pattern"`
				Glob    string `json:"glob"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", err
			}
			re, err := regexp.Compile(args.Pattern)
			if err != nil {
				return "", fmt.Errorf("invalid pattern: %w", err)
			}
			glob := args.Glob
			if glob == "" {
				glob = "*"
			}
			matches, _ := filepath.Glob(filepath.Join(root, glob))

			var results []string
			for _, path := range matches {
				data, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				rel, _ := filepath.Rel(root, path)
				for i, line := range strings.Split(string(data), "\n") {
					if re.MatchString(line) {
						results = append(results, fmt.Sprintf("%s:%d: %s", rel, i+1, line))
					}
				}
			}
			return strings.Join(results, "\n"), nil
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
