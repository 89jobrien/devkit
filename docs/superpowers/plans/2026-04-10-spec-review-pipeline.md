# Spec Review Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `devkit spec [path]` — a six-role concurrent LLM pipeline that reviews spec
markdown files and produces a synthesized verdict, mirroring the council architecture.

**Architecture:** New package `internal/ai/spec` with `Runner`/`RunnerFunc`, `Config`, `Result`,
`Run`, `Synthesize`, `ParseHealthScore`, and `LatestSpecFile`. Command wired in `cmd/devkit/main.go`
as `newSpecCmd`. Config extended with a `[spec]` TOML section for model overrides.

**Tech Stack:** Go, `golang.org/x/sync/errgroup`, `github.com/spf13/cobra`,
`github.com/89jobrien/devkit/internal/ai/providers` (OpenAI), `github.com/stretchr/testify`.

---

### Task 1: Scaffold `internal/ai/spec/spec.go`

**Files:**
- Create: `internal/ai/spec/spec.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ai/spec/spec_test.go`:

```go
package spec_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/89jobrien/devkit/internal/ai/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRunner struct{ response string }

func (s stubRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	return s.response, nil
}

type captureRunner struct {
	mu      sync.Mutex
	prompts []string
}

func (c *captureRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	c.mu.Lock()
	c.prompts = append(c.prompts, prompt)
	c.mu.Unlock()
	return "**Health Score:** 0.75\n**Summary**\nok", nil
}

type errRunner struct{}

func (e errRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	return "", errors.New("provider down")
}

func TestRunAllSixRoles(t *testing.T) {
	runner := stubRunner{response: "**Health Score:** 0.8\n**Summary**\nLooks good."}
	result, err := spec.Run(context.Background(), spec.Config{
		Content: "# My Spec\n\n## Problem\nSomething.\n\n## Design\nStuff.",
		Path:    "docs/superpowers/specs/test.md",
		Runner:  runner,
	})
	require.NoError(t, err)
	assert.Len(t, result.RoleOutputs, 6)
	for _, key := range []string{"completeness", "ambiguity", "scope", "critic", "creative", "generalist"} {
		assert.NotEmpty(t, result.RoleOutputs[key], "missing output for role %q", key)
	}
}

func TestRunNilRunnerReturnsError(t *testing.T) {
	_, err := spec.Run(context.Background(), spec.Config{
		Content: "# Spec",
		Runner:  nil,
	})
	assert.Error(t, err)
}

func TestRunRoleErrorPropagates(t *testing.T) {
	_, err := spec.Run(context.Background(), spec.Config{
		Content: "# Spec",
		Runner:  errRunner{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider down")
}

func TestRunPerRoleOverride(t *testing.T) {
	defaultRunner := stubRunner{response: "**Health Score:** 0.5\n**Summary**\ndefault"}
	capture := &captureRunner{}
	result, err := spec.Run(context.Background(), spec.Config{
		Content: "# Spec",
		Runner:  defaultRunner,
		Runners: map[string]spec.Runner{
			"critic": capture,
		},
	})
	require.NoError(t, err)
	assert.Len(t, capture.prompts, 1, "critic should use override runner")
	assert.NotEmpty(t, result.RoleOutputs["critic"])
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/ai/spec/... 2>&1
```

Expected: FAIL with `cannot find package "github.com/89jobrien/devkit/internal/ai/spec"`

- [ ] **Step 3: Create `internal/ai/spec/spec.go`**

```go
// internal/ai/spec/spec.go
package spec

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Runner is the port for executing LLM calls.
type Runner interface {
	Run(ctx context.Context, prompt string, tools []string) (string, error)
}

// RunnerFunc is a function adapter for Runner.
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

func (f RunnerFunc) Run(ctx context.Context, prompt string, tools []string) (string, error) {
	return f(ctx, prompt, tools)
}

// Config holds parameters for a spec review run.
type Config struct {
	Content         string            // full spec file content
	Path            string            // source path (for display)
	Runner          Runner            // default runner for all six roles
	Runners         map[string]Runner // per-role overrides
	SynthesisRunner Runner            // runner for the synthesis pass
}

// Result holds all role outputs.
type Result struct {
	RoleOutputs map[string]string
}

var roles = map[string]struct{ label, persona string }{
	"completeness": {"Completeness Checker", `You are the COMPLETENESS CHECKER reviewing a software spec document.

Fill in every section of this template. Do not add or remove sections. Replace placeholder
comments with your findings. Cite the spec by section heading (e.g. ## Design) or line number.

**Health Score:** <!-- decimal 0.0–1.0; deduct for each missing section, TBD, or empty placeholder -->

**Summary**
<!-- 2-4 sentences: overall completeness assessment -->

**Missing Sections**
<!-- bullet list: required sections that are absent or empty -->

**Placeholders Found**
<!-- bullet list: TBDs, TODO, "fill in later", vague statements -->

**Recommendations**
<!-- numbered list: concrete additions ordered by priority -->`},

	"ambiguity": {"Ambiguity Detector", `You are the AMBIGUITY DETECTOR reviewing a software spec document.

Fill in every section of this template. Do not add or remove sections. Replace placeholder
comments with your findings. Cite the spec by section heading (e.g. ## Design) or line number.

**Health Score:** <!-- decimal 0.0–1.0; deduct for each requirement that can be read two ways -->

**Summary**
<!-- 2-4 sentences: overall ambiguity assessment -->

**Ambiguous Requirements**
<!-- bullet list: each item as "## Section — [requirement] could mean X or Y" -->

**Unstated Assumptions**
<!-- bullet list: things the spec assumes without stating explicitly -->

**Recommendations**
<!-- numbered list: concrete clarifications ordered by impact -->`},

	"scope": {"Scope Auditor", `You are the SCOPE AUDITOR reviewing a software spec document.

Fill in every section of this template. Do not add or remove sections. Replace placeholder
comments with your findings. Cite the spec by section heading (e.g. ## Design) or line number.

**Health Score:** <!-- decimal 0.0–1.0; deduct for scope creep, over-engineering, or missing decomposition -->

**Summary**
<!-- 2-4 sentences: overall scope assessment -->

**Scope Creep**
<!-- bullet list: features or requirements beyond the stated goal -->

**Over-Engineering Signals**
<!-- bullet list: complexity that is not justified by the requirements -->

**Decomposition Gaps**
<!-- bullet list: areas that should be separate sub-projects or phases -->

**Recommendations**
<!-- numbered list: concrete scope adjustments ordered by priority -->`},

	"critic": {"Strict Critic", `You are the STRICT CRITIC reviewing a software spec document. Be conservative and demanding.
Any critical gap caps Health Score at 0.4.

Fill in every section of this template. Do not add or remove sections. Replace placeholder
comments with your findings. Cite the spec by section heading (e.g. ## Design) or line number.

**Health Score:** <!-- decimal 0.0–1.0; cap at 0.4 if any critical finding -->

**Summary**
<!-- 2-4 sentences: overall verdict -->

**Key Observations**
<!-- bullet list: specific observations tied to spec sections -->

**Risks Identified**
<!-- bullet list: each risk tagged [critical|high|medium|low] -->

**Recommendations**
<!-- numbered list: concrete, actionable, ordered by priority -->`},

	"creative": {"Creative", `You are the CREATIVE reviewer reviewing a software spec document. Be optimistic and inventive.

Fill in every section of this template. Do not add or remove sections. Replace placeholder
comments with your findings. Cite the spec by section heading (e.g. ## Design) or line number.

**Health Score:** <!-- decimal 0.0–1.0 -->

**Summary**
<!-- 2-4 sentences: overall impression -->

**Innovation Opportunities**
<!-- bullet list: creative improvements or extensions the spec enables -->

**Architectural Potential**
<!-- bullet list: design patterns or abstractions worth pursuing -->

**Recommendations**
<!-- numbered list: concrete, actionable, ordered by enthusiasm -->`},

	"generalist": {"Generalist", `You are the GENERALIST reviewing a software spec document. Be balanced and evidence-based.

Fill in every section of this template. Do not add or remove sections. Replace placeholder
comments with your findings. Cite the spec by section heading (e.g. ## Design) or line number.

**Health Score:** <!-- decimal 0.0–1.0 -->

**Summary**
<!-- 2-4 sentences: overall assessment -->

**Progress Indicators**
<!-- bullet list: what the spec gets right, clear requirements, testability signals -->

**Gaps**
<!-- bullet list: missing tests, missing docs, incomplete areas -->

**Recommendations**
<!-- numbered list: concrete, actionable, ordered by impact -->`},
}

var allRoleKeys = []string{"completeness", "ambiguity", "scope", "critic", "creative", "generalist"}

// Run executes all six spec review roles concurrently.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	context_ := fmt.Sprintf("Spec file: %s\n\n```markdown\n%s\n```", cfg.Path, cfg.Content)

	outputs := make(map[string]string, len(allRoleKeys))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	for _, key := range allRoleKeys {
		key := key
		role := roles[key]
		g.Go(func() error {
			prompt := fmt.Sprintf("%s\n\nReview this spec.\n\n%s", role.persona, context_)
			r := cfg.Runner
			if cfg.Runners != nil {
				if override, ok := cfg.Runners[key]; ok {
					r = override
				}
			}
			if r == nil {
				return fmt.Errorf("role %s: no runner configured", key)
			}
			out, err := r.Run(gctx, prompt, nil)
			if err != nil {
				return fmt.Errorf("role %s: %w", key, err)
			}
			mu.Lock()
			outputs[key] = out
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &Result{RoleOutputs: outputs}, nil
}

var healthScoreRe = regexp.MustCompile(`(?i)\*\*Health Score:?\*\*[:\s]*([\d.]+)`)

// ParseHealthScore extracts the first health score from role output text.
func ParseHealthScore(text string) float64 {
	m := healthScoreRe.FindStringSubmatch(text)
	if m == nil {
		return 0.5
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(m[1]), 64)
	return v
}

// MetaScore computes the simple average of all role health scores.
func MetaScore(outputs map[string]string) float64 {
	var sum float64
	var count int
	for _, out := range outputs {
		score := ParseHealthScore(out)
		if score > 0 {
			sum += score
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// Synthesize runs a synthesis pass over all role outputs.
func Synthesize(ctx context.Context, outputs map[string]string, path string, runner Runner) (string, error) {
	var parts []string
	for key, out := range outputs {
		parts = append(parts, fmt.Sprintf("### %s\n%s", roles[key].label, out))
	}
	councilText := strings.Join(parts, "\n\n---\n\n")

	prompt := fmt.Sprintf(`Synthesize this multi-role spec review into a final verdict.

Required sections:
**Health Scores** — list each role's score, then the average as the meta-score.
**Areas of Consensus** — findings where 2+ roles agree.
**Areas of Tension** — "[Role A] sees [X], AND [Role B] sees [Y], suggesting [resolution]."
**Balanced Recommendations** — top 3-5 ranked actions.
**Spec Health** — one of: Ready / Almost / Needs Work — with one-line justification.

Spec file: %s

Role findings:
%s`, path, councilText)

	return runner.Run(ctx, prompt, nil)
}
```

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/ai/spec/... -v 2>&1
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/spec/spec.go internal/ai/spec/spec_test.go
git commit -m "feat(spec): add spec review pipeline package with six roles"
```

---

### Task 2: Add `LatestSpecFile` and its test

**Files:**
- Modify: `internal/ai/spec/spec.go`
- Modify: `internal/ai/spec/spec_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/ai/spec/spec_test.go`:

```go
import (
	// existing imports plus:
	"os"
	"path/filepath"
	"time"
)

func TestLatestSpecFile(t *testing.T) {
	dir := t.TempDir()

	// Write three files with staggered mtimes.
	files := []string{"a.md", "b.md", "c.md"}
	for i, name := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("# "+name), 0o644))
		mtime := time.Now().Add(time.Duration(i) * time.Second)
		require.NoError(t, os.Chtimes(path, mtime, mtime))
	}

	got, err := spec.LatestSpecFile(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "c.md"), got)
}

func TestLatestSpecFileEmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := spec.LatestSpecFile(dir)
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/ai/spec/... -run TestLatestSpecFile -v 2>&1
```

Expected: FAIL with `undefined: spec.LatestSpecFile`

- [ ] **Step 3: Add `LatestSpecFile` to `spec.go`**

Add at the end of `internal/ai/spec/spec.go`:

```go
import (
	// add to existing imports:
	"os"
	"path/filepath"
	"time"
)

// LatestSpecFile returns the path of the most recently modified .md file in dir.
// Returns an error if dir contains no .md files.
func LatestSpecFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("reading spec dir %s: %w", dir, err)
	}

	var latest string
	var latestMod time.Time

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latestMod) {
			latestMod = info.ModTime()
			latest = filepath.Join(dir, e.Name())
		}
	}

	if latest == "" {
		return "", fmt.Errorf("no .md files found in %s", dir)
	}
	return latest, nil
}
```

- [ ] **Step 4: Run all spec tests**

```
go test ./internal/ai/spec/... -v 2>&1
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/spec/spec.go internal/ai/spec/spec_test.go
git commit -m "feat(spec): add LatestSpecFile for auto-discovery"
```

---

### Task 3: Add `ParseHealthScore` and `MetaScore` tests

**Files:**
- Modify: `internal/ai/spec/spec_test.go`

These functions exist from Task 1 but need explicit test coverage.

- [ ] **Step 1: Add tests**

Add to `internal/ai/spec/spec_test.go`:

```go
func TestParseHealthScore(t *testing.T) {
	cases := []struct {
		input    string
		expected float64
	}{
		{"**Health Score:** 0.72", 0.72},
		{"**Health Score:** 1.0", 1.0},
		{"**Health Score:** 0.0", 0.0},
		{"no score here", 0.5},        // default
		{"**health score:** 0.88", 0.88}, // case-insensitive
	}
	for _, c := range cases {
		got := spec.ParseHealthScore(c.input)
		assert.InDelta(t, c.expected, got, 0.001, "input: %q", c.input)
	}
}

func TestMetaScore(t *testing.T) {
	outputs := map[string]string{
		"completeness": "**Health Score:** 0.6",
		"ambiguity":    "**Health Score:** 0.9",
		"scope":        "**Health Score:** 0.8",
		"critic":       "**Health Score:** 0.7",
		"creative":     "**Health Score:** 0.85",
		"generalist":   "**Health Score:** 0.75",
	}
	score := spec.MetaScore(outputs)
	// average: (0.6+0.9+0.8+0.7+0.85+0.75)/6 = 0.7667
	assert.InDelta(t, 0.7667, score, 0.01)
}

func TestSynthesize(t *testing.T) {
	capture := &captureRunner{}
	outputs := map[string]string{
		"completeness": "**Health Score:** 0.6\n**Summary**\nMissing sections.",
		"critic":       "**Health Score:** 0.5\n**Summary**\nCritical gaps.",
	}
	result, err := spec.Synthesize(context.Background(), outputs, "docs/specs/test.md", capture)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	require.Len(t, capture.prompts, 1)
	p := capture.prompts[0]
	assert.Contains(t, p, "Completeness Checker")
	assert.Contains(t, p, "Strict Critic")
	assert.Contains(t, p, "**Health Scores**")
	assert.Contains(t, p, "**Spec Health**")
}
```

- [ ] **Step 2: Run tests**

```
go test ./internal/ai/spec/... -v 2>&1
```

Expected: all 9 tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/ai/spec/spec_test.go
git commit -m "test(spec): add ParseHealthScore, MetaScore, and Synthesize tests"
```

---

### Task 4: Extend `Config` struct with `[spec]` TOML section

**Files:**
- Modify: `cmd/devkit/config.go`
- Modify: `cmd/devkit/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to `cmd/devkit/config_test.go` (check existing file for test structure first):

```go
func TestConfigSpecSection(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, ".devkit.toml")
	content := `
[spec]
role_model      = "gpt-5.4-mini"
synthesis_model = "gpt-5.4"
`
	require.NoError(t, os.WriteFile(tomlPath, []byte(content), 0o644))
	require.NoError(t, os.Chdir(dir))

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "gpt-5.4-mini", cfg.Spec.RoleModel)
	assert.Equal(t, "gpt-5.4", cfg.Spec.SynthesisModel)
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./cmd/devkit/... -run TestConfigSpecSection -v 2>&1
```

Expected: FAIL with `cfg.Spec undefined`

- [ ] **Step 3: Add `Spec` field to `Config` in `config.go`**

In `cmd/devkit/config.go`, add inside the `Config` struct after the `Providers` block:

```go
	Spec struct {
		RoleModel      string `toml:"role_model"`
		SynthesisModel string `toml:"synthesis_model"`
	} `toml:"spec"`
```

- [ ] **Step 4: Run test**

```
go test ./cmd/devkit/... -run TestConfigSpecSection -v 2>&1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/devkit/config.go cmd/devkit/config_test.go
git commit -m "feat(config): add [spec] TOML section for role_model and synthesis_model"
```

---

### Task 5: Wire `newSpecCmd` in `cmd/devkit`

**Files:**
- Create: `cmd/devkit/cmd_spec.go`
- Modify: `cmd/devkit/main.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/devkit/cmd_spec_test.go`:

```go
package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/89jobrien/devkit/internal/ai/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSpecCmdRunsAllRoles(t *testing.T) {
	// Stub runner returns fixed output.
	stub := spec.RunnerFunc(func(_ context.Context, prompt string, _ []string) (string, error) {
		return "**Health Score:** 0.8\n**Summary**\nOK.", nil
	})

	dir := t.TempDir()
	specPath := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(specPath, []byte("# Test Spec\n\n## Problem\nSomething."), 0o644))

	cmd := newSpecCmd(stub, stub)
	cmd.SetArgs([]string{specPath})
	var out strings.Builder
	cmd.SetOut(&out)
	err := cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	body := out.String()
	for _, role := range []string{"completeness", "ambiguity", "scope", "critic", "creative", "generalist"} {
		assert.Contains(t, body, role, "output should contain role %q", role)
	}
	assert.Contains(t, body, "SYNTHESIS")
}

func TestNewSpecCmdAutoDiscovers(t *testing.T) {
	stub := spec.RunnerFunc(func(_ context.Context, _ string, _ []string) (string, error) {
		return "**Health Score:** 0.9\n**Summary**\nGood.", nil
	})

	// Create a temp specs dir and set cwd so discovery finds it.
	dir := t.TempDir()
	specsDir := filepath.Join(dir, "docs", "superpowers", "specs")
	require.NoError(t, os.MkdirAll(specsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(specsDir, "foo.md"), []byte("# Foo"), 0o644))

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(orig) })

	cmd := newSpecCmd(stub, stub)
	var out strings.Builder
	cmd.SetOut(&out)
	err := cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	assert.Contains(t, out.String(), "SYNTHESIS")
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./cmd/devkit/... -run TestNewSpecCmd -v 2>&1
```

Expected: FAIL with `undefined: newSpecCmd`

- [ ] **Step 3: Create `cmd/devkit/cmd_spec.go`**

```go
// cmd/devkit/cmd_spec.go
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/ai/spec"
	"github.com/spf13/cobra"
)

const defaultSpecModel = "gpt-5.4-mini"
const defaultSynthesisModel = "gpt-5.4"
const defaultSpecsDir = "docs/superpowers/specs"

// newSpecCmd returns the spec subcommand. roleRunner and synthRunner are
// injected for testing; pass nil to have the command build them from config.
func newSpecCmd(roleRunner spec.Runner, synthRunner spec.Runner) *cobra.Command {
	var noSynth bool
	cmd := &cobra.Command{
		Use:   "spec [path]",
		Short: "Multi-role spec review",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()

			// Resolve runners from config if not injected.
			rr, sr := roleRunner, synthRunner
			if rr == nil || sr == nil {
				cfg, err := LoadConfig()
				if err != nil {
					return err
				}
				if cfg.Project.Name != "" {
					os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
				}
				roleModel := cfg.Spec.RoleModel
				if roleModel == "" {
					roleModel = defaultSpecModel
				}
				synthModel := cfg.Spec.SynthesisModel
				if synthModel == "" {
					synthModel = defaultSynthesisModel
				}
				apiKey := os.Getenv("OPENAI_API_KEY")
				rr = newOpenAISpecRunner(apiKey, roleModel)
				sr = newOpenAISpecRunner(apiKey, synthModel)
			}

			// Resolve spec path.
			var specPath string
			if len(args) == 1 {
				specPath = args[0]
			} else {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				dir := filepath.Join(wd, defaultSpecsDir)
				specPath, err = spec.LatestSpecFile(dir)
				if err != nil {
					return fmt.Errorf("spec: auto-discover: %w", err)
				}
				fmt.Fprintf(w, "Using latest spec: %s\n\n", specPath)
			}

			content, err := os.ReadFile(specPath)
			if err != nil {
				return fmt.Errorf("spec: read file: %w", err)
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("spec", map[string]string{"path": specPath})
			start := time.Now()

			result, err := spec.Run(cmd.Context(), spec.Config{
				Content: string(content),
				Path:    specPath,
				Runner:  rr,
			})
			if err != nil {
				return err
			}

			var allOutput strings.Builder
			for key, out := range result.RoleOutputs {
				fmt.Fprintf(w, "\n---- %s ----\n%s\n", key, out)
				allOutput.WriteString(fmt.Sprintf("## %s\n%s\n\n", key, out))
			}

			if !noSynth {
				synthesis, err := spec.Synthesize(cmd.Context(), result.RoleOutputs, specPath, sr)
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "\n---- SYNTHESIS ----\n%s\n", synthesis)
				allOutput.WriteString(fmt.Sprintf("## Synthesis\n%s\n", synthesis))
			}

			score := spec.MetaScore(result.RoleOutputs)
			fmt.Fprintf(w, "\nMeta Health Score: %.0f%%\n", score*100)

			devlog.Complete(id, "spec", map[string]string{"path": specPath}, allOutput.String(), time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "spec", allOutput.String(), map[string]string{"path": specPath})
			fmt.Fprintf(w, "\nLogged to: %s\n", path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&noSynth, "no-synthesis", false, "Skip synthesis")
	return cmd
}

// newOpenAISpecRunner constructs a spec.Runner backed by the OpenAI provider.
func newOpenAISpecRunner(apiKey, model string) spec.Runner {
	_ = io.Discard // imported for future streaming; kept for package hygiene
	return spec.RunnerFunc(func(ctx interface{ Done() <-chan struct{} }, prompt string, _ []string) (string, error) {
		// Thin shim: import the providers package and call NewOpenAIProvider.
		// Implemented in the next step.
		panic("replace with real implementation in Step 3 of Task 5")
	})
}
```

**Note:** `newOpenAISpecRunner` above is a placeholder stub to make the file compile structurally. Replace it in the next step with the real implementation using `providers.NewOpenAIProvider`.

- [ ] **Step 4: Replace the `newOpenAISpecRunner` stub with the real implementation**

Edit `cmd/devkit/cmd_spec.go` — replace the `newOpenAISpecRunner` function:

```go
import (
	// add to existing imports:
	"context"
	"github.com/89jobrien/devkit/internal/ai/providers"
)

// newOpenAISpecRunner constructs a spec.Runner backed by the OpenAI provider.
func newOpenAISpecRunner(apiKey, model string) spec.Runner {
	p := providers.NewOpenAIProvider(apiKey, model, "")
	return spec.RunnerFunc(func(ctx context.Context, prompt string, _ []string) (string, error) {
		return p.Chat(ctx, prompt)
	})
}
```

Also remove the `io` import that was added for the stub.

- [ ] **Step 5: Register the command in `main.go`**

In `cmd/devkit/main.go`, add `newSpecCmd(nil, nil)` to the `root.AddCommand(...)` call:

```go
	root.AddCommand(councilCmd, reviewCmd, metaCmd, diagnoseCmd, standupCmd,
		newPrCmd(nil, resolver),
		newChangelogCmd(nil, resolver),
		newLintCmd(nil),
		newExplainCmd(nil, resolver),
		newTestgenCmd(nil, resolver),
		newTicketCmd(nil),
		newAdrCmd(nil),
		newDocgenCmd(nil),
		newMigrateCmd(nil),
		newScaffoldCmd(nil),
		newLogPatternCmd(nil),
		newIncidentCmd(nil),
		newProfileCmd(nil),
		newHealthCmd(nil),
		newAutomateCmd(nil),
		newCITriageCmd(nil),
		newRepoReviewCmd(nil),
		newSpecCmd(nil, nil),
	)
```

- [ ] **Step 6: Build to verify compilation**

```
go build ./cmd/devkit/... 2>&1
```

Expected: no errors.

- [ ] **Step 7: Run all tests**

```
go test ./... 2>&1
```

Expected: all tests PASS (any pre-existing failures are not introduced by this change).

- [ ] **Step 8: Commit**

```bash
git add cmd/devkit/cmd_spec.go cmd/devkit/cmd_spec_test.go cmd/devkit/main.go
git commit -m "feat(cmd): add devkit spec command wired with OpenAI runners"
```

---

### Task 6: Update `.devkit.toml` with `[spec]` section

**Files:**
- Modify: `.devkit.toml`

- [ ] **Step 1: Add `[spec]` section to `.devkit.toml`**

Add at the end of `.devkit.toml`:

```toml
[spec]
role_model      = "gpt-5.4-mini"   # model for the six role agents
synthesis_model = "gpt-5.4"        # model for synthesis
```

- [ ] **Step 2: Verify it loads correctly**

```
go test ./cmd/devkit/... -run TestConfig -v 2>&1
```

Expected: config tests PASS.

- [ ] **Step 3: Reinstall binary**

```
GOBIN=$HOME/go/bin go install ./cmd/devkit ./cmd/ci-agent ./cmd/meta 2>&1
```

Expected: no errors.

- [ ] **Step 4: Smoke test**

```
devkit spec docs/superpowers/specs/2026-04-10-spec-review-pipeline-design.md 2>&1 | head -30
```

Expected: role headers printed, no panic.

- [ ] **Step 5: Commit**

```bash
git add .devkit.toml
git commit -m "chore: add [spec] section to .devkit.toml with default model config"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|---|---|
| `internal/ai/spec` package with `Runner`, `RunnerFunc`, `Config`, `Result` | Task 1 |
| `Run` — all six roles concurrently | Task 1 |
| `Synthesize` | Task 1 |
| `ParseHealthScore`, `MetaScore` | Task 1 + Task 3 |
| `LatestSpecFile` | Task 2 |
| `[spec]` TOML section with `role_model` / `synthesis_model` | Task 4 |
| `devkit spec [path]` command | Task 5 |
| Auto-discover latest spec if no path given | Task 5 |
| Defaults: `gpt-5.4-mini` roles, `gpt-5.4` synthesis | Task 5 + Task 6 |
| No real API calls in tests | Tasks 1-5 (all use stub/capture runners) |

**Placeholder scan:** No TBDs. The `newOpenAISpecRunner` stub in Task 5 Step 3 is intentionally replaced in Step 4 — that sequence is explicit.

**Type consistency:**
- `spec.Runner` interface defined in Task 1, used consistently in Tasks 2-5.
- `spec.Config.Runner` / `spec.Config.SynthesisRunner` — `SynthesisRunner` defined in Task 1 spec but the `Synthesize` function takes `runner Runner` directly (matching council's pattern). The `cmd_spec.go` passes `sr` directly to `Synthesize` — consistent.
- `spec.LatestSpecFile` defined in Task 2, called in Task 5 — signatures match.
