// Package repocontext gathers common repo metadata (name, CLAUDE.md, README, git log)
// used by multiple commands as prompt context for AI calls.
package repocontext

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Context holds the gathered metadata for a repository.
type Context struct {
	// RepoPath is the absolute path to the repository root.
	RepoPath string
	// Name is the base directory name of the repository.
	Name string
	// Claude is the full content of CLAUDE.md, or empty if not present.
	Claude string
	// Readme is the first 2048 bytes of README.md, or empty if not present.
	Readme string
	// GitLog is the output of `git log --oneline -20`, or empty on error.
	GitLog string
}

// Gather reads repo metadata from repoPath. If repoPath is empty, os.Getwd is used.
// Missing files are silently ignored; only the repoPath resolution can return an error.
func Gather(repoPath string) (Context, error) {
	if repoPath == "" {
		var err error
		repoPath, err = os.Getwd()
		if err != nil {
			return Context{}, fmt.Errorf("repocontext: getwd: %w", err)
		}
	}

	ctx := Context{RepoPath: repoPath, Name: filepath.Base(repoPath)}

	if data, err := os.ReadFile(filepath.Join(repoPath, "CLAUDE.md")); err == nil {
		ctx.Claude = string(data)
	}

	if data, err := os.ReadFile(filepath.Join(repoPath, "README.md")); err == nil {
		s := string(data)
		if len(s) > 2048 {
			s = s[:2048]
		}
		ctx.Readme = s
	}

	if out, err := exec.Command("git", "-C", repoPath, "log", "--oneline", "-20").Output(); err == nil {
		ctx.GitLog = string(out)
	}

	return ctx, nil
}

// Summary returns a single-line repo description suitable for use in prompts.
// Format: "repo: <name>"
func (c Context) Summary() string {
	return "repo: " + c.Name
}

// MarkdownSections renders the populated fields as markdown sections.
// Fields with no content are omitted.
func (c Context) MarkdownSections() string {
	var sb strings.Builder
	if c.Claude != "" {
		fmt.Fprintf(&sb, "## CLAUDE.md\n\n%s\n\n", c.Claude)
	}
	if c.Readme != "" {
		fmt.Fprintf(&sb, "## README.md (first 2KB)\n\n%s\n\n", c.Readme)
	}
	if c.GitLog != "" {
		fmt.Fprintf(&sb, "## Recent commits\n\n%s\n\n", c.GitLog)
	}
	return sb.String()
}
