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

// GatherRepoContext returns a markdown snapshot of the current repo state:
// CLAUDE.md/AGENTS.md/README.md previews, recent commits, working tree, and file structure.
// It operates on the current working directory and never returns an error — failures are
// surfaced as empty sections.
func GatherRepoContext() string {
	run := func(args ...string) string {
		out, err := exec.Command(args[0], args[1:]...).Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "devkit: GatherRepoContext: %s: %v\n", strings.Join(args, " "), err)
		}
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
