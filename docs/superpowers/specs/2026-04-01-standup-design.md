# devkit standup — Design Spec

## Goal

`devkit standup` summarizes recent work across one or more git repos into a structured standup update: what I did, what's next, blockers.

## Command Interface

```
devkit standup [--since <duration>] [--repo <path>] [--parallel]
```

| Flag         | Default | Description                                                                                |
| ------------ | ------- | ------------------------------------------------------------------------------------------ |
| `--since`    | `24h`   | Time window for git log and JSONL entries. Accepts Go duration strings (e.g. `8h`, `48h`). |
| `--repo`     | cwd     | Repo path to include. Repeatable — `--repo ~/dev/devkit --repo ~/dev/minibox`.             |
| `--parallel` | false   | Spawn a per-repo summarizer agent for each repo, then synthesize (Option C mode).          |

## Output Format

Plain markdown printed to stdout, logged to `~/.dev-agents/<project>/ai-logs/` via `SaveCommitLog`.

```
## What I did
- …

## What's next
- …

## Blockers
- none
```

## Architecture

### New package: `internal/standup`

Single file `standup.go` with:

```go
type Runner interface {
    Run(ctx context.Context, prompt string, tools []string) (string, error)
}
type RunnerFunc func(ctx context.Context, prompt string, tools []string) (string, error)

type Config struct {
    Repos    []string      // resolved absolute paths; cwd if empty
    Since    time.Duration // default 24h
    Runner   Runner
    Parallel bool
}

func Run(ctx context.Context, cfg Config) (string, error)
```

`Run` dispatches to `runSingle` (one repo) or `runParallel` (multiple repos or `--parallel` flag).

### Data gathering per repo

For each repo path, `gatherRepoContext(path, since)` collects:

1. **Git log:** `git log --since=<duration> --oneline` — commit list
2. **Diff stat:** `git diff --stat HEAD@{<duration>} HEAD` — files changed summary
3. **JSONL entries:** reads `~/.dev-agents/<project>/agent-runs.jsonl`, filters entries where `status == "complete"` and timestamp is within the window — surfaces devkit commands that ran (council, review, diagnose, meta)

Returns a `repoContext` struct:

```go
type repoContext struct {
    Path    string
    Project string
    Commits string
    Stat    string
    Runs    []runEntry // from JSONL
}
```

### Single-repo path (default)

Builds one prompt from `repoContext`, calls `cfg.Runner.Run(ctx, prompt, nil)`, returns result.

**Prompt structure:**

```
You are generating a standup update for a software engineer.

Project: <name>
Time window: last <N> hours

Recent commits:
<git log>

Changed files:
<diff stat>

Devkit runs:
<jsonl entries: command, duration, health scores if council>

Produce a standup update with exactly three sections:
## What I did
## What's next
## Blockers

Be concise. Infer "what's next" from incomplete work and commit messages. If no blockers are evident, write "none".
```

### Parallel path (`--parallel` or multiple `--repo` flags)

Mirrors `internal/meta` pattern:

1. Spawn one goroutine per repo via `errgroup`
2. Each goroutine calls `cfg.Runner.Run` with a per-repo prompt asking for a brief summary (not the full standup format)
3. Collect per-repo summaries
4. Call `cfg.Runner.Run` with a synthesis prompt that combines all summaries into the final three-section standup

### Wiring in `cmd/devkit/main.go`

New `standup` subcommand:

```go
var standupSince string
var standupRepos []string
var standupParallel bool
standupCmd := &cobra.Command{
    Use:   "standup",
    Short: "Summarize recent work as a standup update",
    RunE: func(cmd *cobra.Command, args []string) error { ... },
}
standupCmd.Flags().StringVar(&standupSince, "since", "24h", "Time window (Go duration, e.g. 24h, 8h)")
standupCmd.Flags().StringArrayVar(&standupRepos, "repo", nil, "Repo paths to include (repeatable, defaults to cwd)")
standupCmd.Flags().BoolVar(&standupParallel, "parallel", false, "Summarize repos in parallel then synthesize")
```

Duration is parsed with `time.ParseDuration`. Invalid durations return an error.

Repos default to `[]string{cwd}` if `--repo` is not set.

Uses `router.For(providers.TierBalanced)` — no tool use needed (all context is gathered upfront).

Logs result via `devlog.SaveCommitLog`.

## Error Handling

- Repo path does not exist or is not a git repo: skip with a warning, continue with remaining repos
- No commits in window: include repo in context with "no commits in window" — let the LLM handle it gracefully
- JSONL file missing: skip silently (normal for repos that haven't run devkit yet)
- `git diff HEAD@{<duration>}` fails (shallow clone, no prior ref): fall back to `git diff --stat` with no range

## Testing

- `internal/standup/standup_test.go` — stub runner, assert prompt contains git log and JSONL entries
- Test `gatherRepoContext` with a temp git repo (init, commit, check output)
- Test parallel path: two stub repos, assert synthesis prompt contains both summaries
- No real API calls in unit tests

## Files

| Path                               | Action                          |
| ---------------------------------- | ------------------------------- |
| `internal/standup/standup.go`      | Create                          |
| `internal/standup/standup_test.go` | Create                          |
| `cmd/devkit/main.go`               | Modify — add standup subcommand |
