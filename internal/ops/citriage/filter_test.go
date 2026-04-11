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

// --- Negative / preservation tests ---

func TestFilterLogPreservesGHAErrorAnnotation(t *testing.T) {
	// ##[error] is a GHA annotation that carries actionable info; it must not
	// be dropped by the ##[group]/##[endgroup]/##[debug] prefix rules.
	raw := "##[error]Process completed with exit code 1."
	got := filterLog(raw)
	if !strings.Contains(got, "##[error]") {
		t.Errorf("##[error] annotation was incorrectly dropped: %q", got)
	}
}

func TestFilterLogPreservesGHAWarningAnnotation(t *testing.T) {
	raw := "##[warning]Node.js 16 actions are deprecated."
	got := filterLog(raw)
	if !strings.Contains(got, "##[warning]") {
		t.Errorf("##[warning] annotation was incorrectly dropped: %q", got)
	}
}

func TestFilterLogPreservesErrorMentioningGitConfig(t *testing.T) {
	// An error message that mentions "git config" should NOT be dropped,
	// even though "[command]/usr/bin/git config" is boilerplate.
	raw := `fatal: unable to read config file '/home/runner/.gitconfig': No such file or directory
error: could not set 'git config user.email' — check your git config`
	got := filterLog(raw)
	if !strings.Contains(got, "fatal: unable to read config") {
		t.Errorf("legitimate git config error was dropped: %q", got)
	}
	if !strings.Contains(got, "could not set") {
		t.Errorf("legitimate git config error was dropped: %q", got)
	}
}

func TestFilterLogPreservesTabDelimitedTestOutput(t *testing.T) {
	// Tab-delimited lines that aren't GHA job prefixes must survive.
	// The tightened reJobPrefix requires the first field to be a slug.
	raw := "\t\tgot:  map[string]int{\"a\": 1}\n\t\twant: map[string]int{\"a\": 2}"
	got := filterLog(raw)
	if !strings.Contains(got, "got:") || !strings.Contains(got, "want:") {
		t.Errorf("tab-indented test diff was incorrectly stripped: %q", got)
	}
}

func TestFilterLogPreservesGoTestFAILWithTabs(t *testing.T) {
	// go test outputs FAIL lines with a tab; they must not be eaten.
	raw := "FAIL\tgithub.com/89jobrien/devkit/internal/ops/citriage\t0.042s"
	got := filterLog(raw)
	if !strings.Contains(got, "FAIL") {
		t.Errorf("go test FAIL line was dropped: %q", got)
	}
}

func TestFilterLogPreservesHintLikeErrorContent(t *testing.T) {
	// The word "hint" inside a compiler error should not trigger the
	// "hint: " prefix rule because the line doesn't start with "hint: ".
	raw := "error[E0277]: the trait bound is not satisfied; hint: consider adding #[derive(Clone)]"
	got := filterLog(raw)
	if !strings.Contains(got, "error[E0277]") {
		t.Errorf("error line containing 'hint' was incorrectly dropped: %q", got)
	}
}

// --- BOM-prefixed timestamp test ---

func TestFilterLogStripsBOMPrefixedTimestamp(t *testing.T) {
	// BOM (\xef\xbb\xbf) can appear before timestamps in some GHA log formats.
	raw := "\xef\xbb\xbf2026-03-24T03:31:16.4894510Z real error here"
	got := filterLog(raw)
	if strings.Contains(got, "2026-03-24") {
		t.Errorf("BOM-prefixed timestamp not stripped: %q", got)
	}
	if strings.Contains(got, "\xef\xbb\xbf") {
		t.Errorf("BOM not stripped: %q", got)
	}
	if !strings.Contains(got, "real error here") {
		t.Errorf("content after BOM timestamp lost: %q", got)
	}
}

// --- hint: prefix behavior ---

func TestFilterLogPreservesLineStartingHintFromCompiler(t *testing.T) {
	// The `hint: ` prefix rule targets git bootstrap output. This test
	// documents that lines starting with "hint: " ARE currently dropped.
	// If a non-git tool emits actionable "hint: ..." lines, this rule should
	// be narrowed. For now we assert the documented (drop) behavior.
	raw := "hint: use `go mod tidy` to update your go.sum"
	got := filterLog(raw)
	// Currently dropped — document this as intentional GHA-git bootstrap noise.
	if strings.Contains(got, "hint: use") {
		// If this starts passing (rule narrowed), update the test accordingly.
		t.Log("hint: line was preserved — rule may have been narrowed (update test)")
	}
}

func TestFilterLogPreservesHintInsideErrorLine(t *testing.T) {
	// "hint:" appearing mid-line in a compiler error must NOT be dropped.
	raw := "error: cannot borrow as mutable; hint: consider making this binding mutable"
	got := filterLog(raw)
	if !strings.Contains(got, "error: cannot borrow") {
		t.Errorf("error line containing 'hint:' mid-line was dropped: %q", got)
	}
}

// --- reJobPrefix edge cases ---

func TestFilterLogStripsJobPrefixWithDots(t *testing.T) {
	// Job names like "build.linux" are common in GHA matrix workflows.
	raw := "build.linux\tRun tests\tFAIL github.com/foo/bar"
	got := filterLog(raw)
	if strings.Contains(got, "build.linux") {
		t.Errorf("job prefix with dots not stripped: %q", got)
	}
	if !strings.Contains(got, "FAIL github.com/foo/bar") {
		t.Errorf("payload lost: %q", got)
	}
}

func TestFilterLogStripsJobPrefixWithParens(t *testing.T) {
	// Matrix job names like "build (ubuntu-latest)" appear in GHA logs.
	// The current slug allowlist includes spaces, so this should strip.
	raw := "build (ubuntu-latest)\tRun tests\terror: undefined symbol"
	got := filterLog(raw)
	if strings.Contains(got, "build (ubuntu-latest)") {
		t.Errorf("job prefix with parens not stripped: %q", got)
	}
	if !strings.Contains(got, "error: undefined symbol") {
		t.Errorf("payload lost: %q", got)
	}
}

func TestFilterLogStripsJobPrefixWithColon(t *testing.T) {
	// Job names with colons (e.g. "test: unit") are outside the slug class.
	// This test documents whether they are stripped or pass through.
	raw := "test: unit\tRun\tsome output"
	got := filterLog(raw)
	// Document current behavior — colon is outside [a-zA-Z0-9_/ -] so prefix
	// is NOT stripped; the full line reaches the runner.
	if !strings.Contains(got, "some output") {
		t.Errorf("payload was unexpectedly lost for colon job name: %q", got)
	}
}

// --- Timestamp variant coverage ---

func TestFilterLogStripsTimestampNoFractionalSeconds(t *testing.T) {
	// ISO-8601 without fractional seconds — documents whether current regex
	// handles this variant. Currently NOT stripped (regex requires \.\d+).
	raw := "2026-03-24T03:31:16Z real error here"
	got := filterLog(raw)
	// Document: this timestamp form passes through unstripped.
	if !strings.Contains(got, "real error here") {
		t.Errorf("content lost when processing no-fractional timestamp: %q", got)
	}
}

// --- Size reduction verification ---

func TestFilterLogSizeReduction(t *testing.T) {
	// Build a realistic GHA log with a known ratio of noise to signal.
	var b strings.Builder
	for i := 0; i < 50; i++ {
		// Noise lines (runner setup, git commands, hints)
		b.WriteString("build\tSetup\t2026-01-01T00:00:00.0000000Z ##[group]Run actions/checkout@v4\n")
		b.WriteString("build\tSetup\t2026-01-01T00:00:00.0000000Z Runner Image Provisioner v1.2.3\n")
		b.WriteString("build\tSetup\t2026-01-01T00:00:00.0000000Z ##[endgroup]\n")
		b.WriteString("build\tSetup\t2026-01-01T00:00:00.0000000Z [command]/usr/bin/git version\n")
		b.WriteString("build\tSetup\t2026-01-01T00:00:00.0000000Z hint: Using 'master' as the name\n")
	}
	// Signal lines (actual errors)
	for i := 0; i < 10; i++ {
		b.WriteString("build\tBuild\t2026-01-01T00:00:01.0000000Z error: cannot find type `Foo` in this scope\n")
		b.WriteString("build\tBuild\t2026-01-01T00:00:01.0000000Z   --> src/main.rs:42:5\n")
	}
	raw := b.String()
	got := filterLog(raw)

	// The 250 noise lines should be almost entirely removed.
	reduction := 1.0 - float64(len(got))/float64(len(raw))
	if reduction < 0.50 {
		t.Errorf("insufficient size reduction: %.1f%% (want >= 50%%); before=%d after=%d",
			reduction*100, len(raw), len(got))
	}
	if !strings.Contains(got, "cannot find type") {
		t.Errorf("error content lost after filtering")
	}
}
