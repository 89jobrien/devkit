package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	devgit "github.com/89jobrien/devkit/internal/infra/git"
)

// Compile-time check: ExecRangeResolver satisfies RangeResolver.
var _ devgit.RangeResolver = devgit.ExecRangeResolver{}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "ci-test-user@localhost"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", args, err, b)
		}
	}
	return dir
}

func commit(t *testing.T, dir, msg string) {
	t.Helper()
	f := filepath.Join(dir, msg+".txt")
	if err := os.WriteFile(f, []byte(msg), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", msg},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("commit %v: %v\n%s", args, err, b)
		}
	}
}

func TestResolveRange_NormalBranch(t *testing.T) {
	dir := initRepo(t)
	commit(t, dir, "initial")

	cmd := exec.Command("git", "checkout", "-b", "feature")
	cmd.Dir = dir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout: %v\n%s", err, b)
	}
	commit(t, dir, "feature-change")

	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	r, err := devgit.ExecRangeResolver{}.ResolveRange("main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Fallback {
		t.Error("expected Fallback=false on normal branch")
	}
	if !strings.HasSuffix(r.Range, "...HEAD") {
		t.Errorf("expected range ending in ...HEAD, got: %s", r.Range)
	}
}

func TestResolveRange_MainAtBase(t *testing.T) {
	dir := initRepo(t)
	commit(t, dir, "first")
	commit(t, dir, "second")

	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	r, err := devgit.ExecRangeResolver{}.ResolveRange("main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Fallback {
		t.Error("expected Fallback=true when on main at base")
	}
	if r.Range != "HEAD~1...HEAD" {
		t.Errorf("expected HEAD~1...HEAD, got: %s", r.Range)
	}
}

func TestResolveRange_SingleCommit(t *testing.T) {
	dir := initRepo(t)
	commit(t, dir, "only-commit")

	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	_, err := devgit.ExecRangeResolver{}.ResolveRange("main")
	if err == nil {
		t.Error("expected error on single-commit repo, got nil")
	}
	if !strings.Contains(err.Error(), "no parent commit") {
		t.Errorf("expected 'no parent commit' in error, got: %v", err)
	}
}

func TestResolveRange_DetachedHEAD(t *testing.T) {
	dir := initRepo(t)
	commit(t, dir, "first")
	commit(t, dir, "second")

	cmd := exec.Command("git", "checkout", "--detach", "HEAD")
	cmd.Dir = dir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("detach HEAD: %v\n%s", err, b)
	}

	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	r, err := devgit.ExecRangeResolver{}.ResolveRange("main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Fallback {
		t.Error("expected Fallback=true for detached HEAD at base")
	}
	if r.Range != "HEAD~1...HEAD" {
		t.Errorf("expected HEAD~1...HEAD, got: %s", r.Range)
	}
}
