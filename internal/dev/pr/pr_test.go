// internal/pr/pr_test.go
package pr

import (
	"context"
	"strings"
	"testing"
)

type stubRunner struct {
	calls  []string
	result string
}

func (s *stubRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	s.calls = append(s.calls, prompt)
	return s.result, nil
}

func TestRunPromptContainsDiffAndLog(t *testing.T) {
	stub := &stubRunner{result: "# My PR\n\nDid stuff.\n\n## Changes\n- thing\n\n## Test Plan\n- run tests"}

	result, err := Run(context.Background(), Config{
		Base:   "main",
		Diff:   "diff --git a/foo.go b/foo.go\n+added line",
		Log:    "abc1234 add feature",
		Stat:   "foo.go | 1 +",
		Runner: stub,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 runner call, got %d", len(stub.calls))
	}
	prompt := stub.calls[0]

	for _, want := range []string{
		"main",
		"abc1234 add feature",
		"foo.go | 1 +",
		"diff --git",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestRunTruncatesLargeDiff(t *testing.T) {
	bigDiff := strings.Repeat("x", maxDiffBytes+100)
	stub := &stubRunner{result: "# PR\n\nSummary.\n\n## Changes\n- a\n\n## Test Plan\n- b"}

	_, err := Run(context.Background(), Config{
		Base:   "main",
		Diff:   bigDiff,
		Runner: stub,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prompt := stub.calls[0]
	if !strings.Contains(prompt, "[diff truncated]") {
		t.Error("expected [diff truncated] in prompt for oversized diff")
	}
	if strings.Contains(prompt, bigDiff) {
		t.Error("prompt should not contain the full oversized diff")
	}
}

func TestRunNoCommitsGraceful(t *testing.T) {
	stub := &stubRunner{result: "# Empty PR\n\nNo changes.\n\n## Changes\n- none\n\n## Test Plan\n- n/a"}

	_, err := Run(context.Background(), Config{
		Base:   "main",
		Diff:   "",
		Log:    "",
		Stat:   "",
		Runner: stub,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prompt := stub.calls[0]
	if !strings.Contains(prompt, "(no commits in range)") {
		t.Errorf("expected graceful no-commits message in prompt:\n%s", prompt)
	}
}

func TestResolveBaseExplicitOverride(t *testing.T) {
	got := ResolveBase("feature/my-branch")
	if got != "feature/my-branch" {
		t.Errorf("expected 'feature/my-branch', got %q", got)
	}
}

func TestResolveBaseExplicitDevelop(t *testing.T) {
	got := ResolveBase("develop")
	if got != "develop" {
		t.Errorf("expected 'develop', got %q", got)
	}
}
