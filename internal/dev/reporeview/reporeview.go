// Package reporeview runs a council-style review scoped to overall repo health.
package reporeview

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/89jobrien/devkit/internal/ai/council"
	"github.com/89jobrien/devkit/internal/repocontext"
)

// Config holds inputs for a repo-review run.
type Config struct {
	RepoPath string
	Runner   council.Runner
	Format   string // "markdown" (default) or "json"
}

// Run gathers repo context and runs a council review against it.
func Run(ctx context.Context, cfg Config) (string, error) {
	if cfg.Runner == nil {
		return "", fmt.Errorf("reporeview: runner is required")
	}

	rc, err := repocontext.Gather(cfg.RepoPath)
	if err != nil {
		return "", fmt.Errorf("reporeview: %w", err)
	}

	prompt := buildPrompt(rc)
	output, err := cfg.Runner.Run(ctx, prompt, nil)
	if err != nil {
		return "", err
	}
	if cfg.Format == "json" {
		b, jerr := json.Marshal(map[string]string{"output": output})
		if jerr != nil {
			return "", fmt.Errorf("reporeview: json marshal: %w", jerr)
		}
		return string(b), nil
	}
	return output, nil
}


func dirTree(root string, depth int) string {
	var sb strings.Builder
	_ = walkDir(root, root, 0, depth, &sb)
	return sb.String()
}

func walkDir(root, path string, current, max int, sb *strings.Builder) error {
	if current > max {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || e.Name() == "vendor" || e.Name() == "node_modules" {
			continue
		}
		rel, _ := filepath.Rel(root, filepath.Join(path, e.Name()))
		indent := strings.Repeat("  ", current)
		if e.IsDir() {
			fmt.Fprintf(sb, "%s%s/\n", indent, rel)
			_ = walkDir(root, filepath.Join(path, e.Name()), current+1, max, sb)
		} else {
			fmt.Fprintf(sb, "%s%s\n", indent, rel)
		}
	}
	return nil
}

func buildPrompt(rc repocontext.Context) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "You are reviewing the repository %q as a senior engineer.\n\n", rc.Name)
	sb.WriteString("Identify the top issues by priority. What needs attention in this repo?\n\n")
	sb.WriteString(rc.MarkdownSections())
	if tree := dirTree(rc.RepoPath, 2); tree != "" {
		fmt.Fprintf(&sb, "## Directory structure\n\n%s\n\n", tree)
	}
	sb.WriteString("Provide a prioritized list of issues with actionable recommendations.\n")
	return sb.String()
}
