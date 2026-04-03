// Package git provides a port and adapter for git revision-range operations.
package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// RangeResult is the resolved git revision range for diff/log/stat operations.
type RangeResult struct {
	Range    string // e.g. "HEAD~1...HEAD" or "main...HEAD"
	Fallback bool   // true when base==HEAD and HEAD~1 path was taken
}

// RangeResolver is the port for resolving a git revision range.
type RangeResolver interface {
	ResolveRange(base string) (RangeResult, error)
}

// ExecRangeResolver implements RangeResolver using git subprocesses.
type ExecRangeResolver struct{}

// ResolveRange resolves base against HEAD and returns the effective range.
//
// Resolution logic:
//  1. Resolve base and HEAD to SHAs. Any subprocess failure is returned as an error.
//  2. If SHAs differ → {Range: base+"...HEAD", Fallback: false}.
//  3. If SHAs are equal (base == HEAD) → verify HEAD~1 exists.
//     If it does → {Range: "HEAD~1...HEAD", Fallback: true}.
//     If it does not → error ("no parent commit: single-commit repository").
func (ExecRangeResolver) ResolveRange(base string) (RangeResult, error) {
	baseOut, err := exec.Command("git", "rev-parse", base).Output()
	if err != nil {
		return RangeResult{}, fmt.Errorf("git: resolve base %q: %w", base, err)
	}
	headOut, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return RangeResult{}, fmt.Errorf("git: resolve HEAD: %w", err)
	}

	baseSHA := strings.TrimSpace(string(baseOut))
	headSHA := strings.TrimSpace(string(headOut))

	if baseSHA != headSHA {
		return RangeResult{Range: base + "...HEAD", Fallback: false}, nil
	}

	// base == HEAD: fall back to HEAD~1 if a parent exists.
	if err := exec.Command("git", "rev-parse", "--verify", "HEAD~1").Run(); err != nil {
		return RangeResult{}, fmt.Errorf("git: no parent commit: single-commit repository")
	}
	return RangeResult{Range: "HEAD~1...HEAD", Fallback: true}, nil
}

// Diff returns the output of `git diff <r.Range>`.
func Diff(r RangeResult) (string, error) {
	out, err := exec.Command("git", "diff", r.Range).Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s: %w", r.Range, err)
	}
	return string(out), nil
}

// Log returns the output of `git log <r.Range> --oneline`.
func Log(r RangeResult) (string, error) {
	out, err := exec.Command("git", "log", r.Range, "--oneline").Output()
	if err != nil {
		return "", fmt.Errorf("git log %s: %w", r.Range, err)
	}
	return string(out), nil
}

// Stat returns the output of `git diff <r.Range> --stat`.
func Stat(r RangeResult) (string, error) {
	out, err := exec.Command("git", "diff", r.Range, "--stat").Output()
	if err != nil {
		return "", fmt.Errorf("git diff --stat %s: %w", r.Range, err)
	}
	return string(out), nil
}
