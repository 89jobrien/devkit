package git_test

import (
	"testing"

	devgit "github.com/89jobrien/devkit/internal/infra/git"
)

// Compile-time check: ExecRangeResolver satisfies RangeResolver.
var _ devgit.RangeResolver = devgit.ExecRangeResolver{}

func TestRangeResult_Fields(t *testing.T) {
	r := devgit.RangeResult{Range: "main...HEAD", Fallback: false}
	if r.Range != "main...HEAD" {
		t.Errorf("unexpected Range: %s", r.Range)
	}
	if r.Fallback {
		t.Error("Fallback should be false")
	}
}
