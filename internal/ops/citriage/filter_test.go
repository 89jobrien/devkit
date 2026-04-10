package citriage

import (
	"strings"
	"testing"
)

func TestFilterLogStripsTimestamps(t *testing.T) {
	raw := "2026-03-24T03:31:16.4894510Z some error here"
	got := filterLog(raw)
	if strings.Contains(got, "2026-03-24") {
		t.Errorf("timestamp not stripped: %q", got)
	}
	if !strings.Contains(got, "some error here") {
		t.Errorf("content lost: %q", got)
	}
}

func TestFilterLogStripsANSI(t *testing.T) {
	raw := "\x1b[92mINFO\x1b[0m loaded"
	got := filterLog(raw)
	if strings.Contains(got, "\x1b") {
		t.Errorf("ANSI not stripped: %q", got)
	}
	if !strings.Contains(got, "loaded") {
		t.Errorf("content lost: %q", got)
	}
}

func TestFilterLogStripsJobPrefix(t *testing.T) {
	raw := "council\tUNKNOWN STEP\tsome log line"
	got := filterLog(raw)
	if strings.Contains(got, "council") {
		t.Errorf("job prefix not stripped: %q", got)
	}
	if !strings.Contains(got, "some log line") {
		t.Errorf("content lost: %q", got)
	}
}

func TestFilterLogDropsBoilerplate(t *testing.T) {
	raw := "##[group]Setting up Go\n##[endgroup]\nsome real error"
	got := filterLog(raw)
	if strings.Contains(got, "##[group]") || strings.Contains(got, "##[endgroup]") {
		t.Errorf("boilerplate not dropped: %q", got)
	}
	if !strings.Contains(got, "some real error") {
		t.Errorf("content lost: %q", got)
	}
}

func TestFilterLogCollapsesBlankLines(t *testing.T) {
	raw := "line1\n\n\n\nline2"
	got := filterLog(raw)
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("consecutive blanks not collapsed: %q", got)
	}
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") {
		t.Errorf("content lost: %q", got)
	}
}

func TestFilterLogReducesRealGHAOutput(t *testing.T) {
	// Simulate a typical gh run view --log-failed output block.
	raw := `council	UNKNOWN STEP	2026-03-24T03:31:16.4894510Z ##[group]Runner Image Provisioner
council	UNKNOWN STEP	2026-03-24T03:31:16.4921020Z Hosted Compute Agent
council	UNKNOWN STEP	2026-03-24T03:31:16.4929884Z ##[endgroup]
council	build	2026-03-24T03:32:01.1234567Z FAIL	github.com/89jobrien/devkit/internal/ops/citriage [build failed]
council	build	2026-03-24T03:32:01.1234568Z ./citriage.go:42:3: undefined: filterLog`
	got := filterLog(raw)
	if strings.Contains(got, "Runner Image Provisioner") {
		t.Errorf("boilerplate not removed")
	}
	if !strings.Contains(got, "undefined: filterLog") {
		t.Errorf("error line lost: %q", got)
	}
	if len(got) >= len(raw) {
		t.Errorf("filter did not reduce size: before=%d after=%d", len(raw), len(got))
	}
}
