// cmd/devkit/main.go
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/89jobrien/devkit/internal/ai/baml"
	"github.com/89jobrien/devkit/internal/ai/council"
	"github.com/89jobrien/devkit/internal/ops/diagnose"
	devgit "github.com/89jobrien/devkit/internal/infra/git"
	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/ai/meta"
	"github.com/89jobrien/devkit/internal/repocontext"
	"github.com/89jobrien/devkit/internal/ai/providers"
	"github.com/89jobrien/devkit/internal/dev/review"
	"github.com/89jobrien/devkit/internal/ops/standup"
	"github.com/89jobrien/devkit/internal/infra/tools"
	"github.com/spf13/cobra"
)

// resolveDiffBase returns the most recent git tag if one exists and is reachable,
// otherwise falls back to "main". Commands that need a consistent default base
// should call this rather than hardcoding "main".
func resolveDiffBase() string {
	out, err := exec.Command("git", "describe", "--tags", "--abbrev=0").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return "main"
	}
	tag := strings.TrimSpace(string(out))
	// Verify the tag resolves to an actual object so we don't silently use a broken ref.
	if err := exec.Command("git", "rev-parse", "--verify", tag).Run(); err != nil {
		return "main"
	}
	return tag
}

// resolveChangelogBase is an alias kept for clarity at the changelog call site.
func resolveChangelogBase() string { return resolveDiffBase() }

// validateRef returns an error if the ref cannot be resolved in the local repo.
func validateRef(ref string) error {
	if err := exec.Command("git", "rev-parse", "--verify", ref).Run(); err != nil {
		return fmt.Errorf("base ref %q not found in local repository", ref)
	}
	return nil
}

// agentToolsAt returns the standard set of code-intelligence tools rooted at wd.
func agentToolsAt(wd string) []tools.Tool {
	return []tools.Tool{
		tools.ReadTool(wd),
		tools.GlobTool(wd),
		tools.GrepTool(wd),
	}
}

var sdkDocURLs = []string{
	"https://docs.anthropic.com/en/docs/claude-code/sdk",
	"https://docs.anthropic.com/en/docs/claude-code/sdk/sdk-python",
}

func fetchSDKDocs(forceRefresh bool) string {
	home, _ := os.UserHomeDir()
	cacheDir := home + "/.dev-agents/cache"
	cachePath := cacheDir + "/sdk-docs.md"

	if !forceRefresh {
		if info, err := os.Stat(cachePath); err == nil {
			if time.Since(info.ModTime()) < 24*time.Hour {
				data, _ := os.ReadFile(cachePath)
				return string(data)
			}
		}
	}

	_ = os.MkdirAll(cacheDir, 0o755)

	var sections []string
	client := &http.Client{Timeout: 15 * time.Second}
	for _, url := range sdkDocURLs {
		resp, err := client.Get(url)
		if err != nil {
			sections = append(sections, fmt.Sprintf("<!-- failed to fetch %s: %v -->", url, err))
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		text := htmlToText(string(raw))
		if len(text) > 20000 {
			text = text[:20000]
		}
		sections = append(sections, fmt.Sprintf("<!-- %s -->\n%s", url, text))
	}

	content := strings.Join(sections, "\n\n---\n\n")
	_ = os.WriteFile(cachePath, []byte(content), 0o644)
	return content
}

func htmlToText(html string) string {
	re := regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	html = re.ReplaceAllString(html, "")
	tags := regexp.MustCompile(`<[^>]+>`)
	text := tags.ReplaceAllString(html, " ")
	ws := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(ws.ReplaceAllString(text, " "))
}

func main() {
	root := &cobra.Command{
		Use:   "devkit",
		Short: "AI-powered dev workflow toolkit",
	}

	resolver := devgit.ExecRangeResolver{}

	// council subcommand
	var councilBase, councilMode string
	var councilNoSynth bool
	councilCmd := &cobra.Command{
		Use:   "council",
		Short: "Multi-role branch analysis",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			rangeResult, err := resolver.ResolveRange(councilBase)
			if err != nil {
				return fmt.Errorf("council: resolve git range: %w", err)
			}
			diff, err := devgit.Diff(rangeResult)
			if err != nil {
				return fmt.Errorf("council: git diff: %w", err)
			}
			commits, err := devgit.Log(rangeResult)
			if err != nil {
				return fmt.Errorf("council: git log: %w", err)
			}
			stat, err := devgit.Stat(rangeResult)
			if err != nil {
				return fmt.Errorf("council: git stat: %w", err)
			}

			if strings.TrimSpace(diff) == "" {
				return fmt.Errorf("no diff found vs %q — nothing to review (run `git log %s..HEAD --oneline` to verify commits exist)", councilBase, councilBase)
			}

			router, err := newRouterFromConfig(cfg)
			if err != nil {
				return err
			}

			// Build per-role runners based on semantic tier routing.
			roleRunners := make(map[string]council.Runner)
			for _, role := range []string{"creative-explorer", "performance-analyst", "general-analyst", "security-reviewer", "strict-critic"} {
				if cfg.Providers.UseBAML {
					roleRunners[role] = baml.New(role, os.Stdout)
				} else {
					tier := providers.TierForRole(role)
					roleRunners[role] = router.For(tier)
				}
			}

			councilCfg := council.Config{
				Base:    councilBase,
				Mode:    councilMode,
				Diff:    diff,
				Commits: commits,
				Stat:    stat,
				Runner:  router.For(providers.TierBalanced), // default for unrecognized roles
				Runners: roleRunners,
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("council", map[string]string{"base": councilBase, "mode": councilMode})
			start := time.Now()

			result, err := council.Run(cmd.Context(), councilCfg)
			if err != nil {
				return err
			}

			var allOutput strings.Builder
			for key, out := range result.RoleOutputs {
				fmt.Printf("\n---- %s ----\n%s\n", key, out)
				allOutput.WriteString(fmt.Sprintf("## %s\n%s\n\n", key, out))
			}

			if !councilNoSynth {
				synthesis, err := council.Synthesize(cmd.Context(), result.RoleOutputs, council.Config{
					Base: councilBase, Diff: diff, Commits: commits,
				}, router.For(providers.TierBalanced))
				if err != nil {
					return err
				}
				fmt.Printf("\n---- SYNTHESIS ----\n%s\n", synthesis)
				allOutput.WriteString(fmt.Sprintf("## Synthesis\n%s\n", synthesis))
			}

			score := council.MetaScore(result.RoleOutputs)
			fmt.Printf("\nMeta Health Score: %.0f%%\n", score*100)

			devlog.Complete(id, "council", map[string]string{"base": councilBase, "mode": councilMode},
				allOutput.String(), time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "council", allOutput.String(), map[string]string{
				"base": councilBase, "mode": councilMode,
			})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	councilCmd.Flags().StringVar(&councilBase, "base", "main", "Base branch/ref")
	councilCmd.Flags().StringVar(&councilMode, "mode", "core", "core or extensive")
	councilCmd.Flags().BoolVar(&councilNoSynth, "no-synthesis", false, "Skip synthesis")

	// review subcommand
	var reviewBase string
	reviewCmd := &cobra.Command{
		Use:   "review",
		Short: "AI diff review",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			rangeResult, err := resolver.ResolveRange(reviewBase)
			if err != nil {
				return fmt.Errorf("review: resolve git range: %w", err)
			}
			diff, err := devgit.Diff(rangeResult)
			if err != nil {
				return fmt.Errorf("review: git diff: %w", err)
			}
			if strings.TrimSpace(diff) == "" {
				fmt.Printf("No changes versus %s — nothing to review.\n", reviewBase)
				return nil
			}

			router, err := newRouterFromConfig(cfg)
			if err != nil {
				return err
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("review", map[string]string{"base": reviewBase})
			start := time.Now()

			result, err := review.Run(cmd.Context(), review.Config{
				Base:  reviewBase,
				Diff:  diff,
				Focus: cfg.Review.Focus,
				Runner: review.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					wd, _ := os.Getwd()
					agentTools := []tools.Tool{
						tools.ReadTool(wd),
						tools.GlobTool(wd),
						tools.GrepTool(wd),
					}
					return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
				}),
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "review", map[string]string{"base": reviewBase}, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "review", result, map[string]string{"base": reviewBase})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	reviewCmd.Flags().StringVar(&reviewBase, "base", "main", "Base branch/ref")

	// meta subcommand
	var metaNoSynth, metaRefreshDocs bool
	metaCmd := &cobra.Command{
		Use:   "meta [task]",
		Short: "Design and run parallel agents for any task",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			task := ""
			if len(args) > 0 {
				task = args[0]
			} else {
				info, _ := os.Stdin.Stat()
				if info.Mode()&os.ModeCharDevice == 0 {
					var sb strings.Builder
					buf := make([]byte, 4096)
					for {
						n, err := os.Stdin.Read(buf)
						sb.Write(buf[:n])
						if err != nil {
							break
						}
					}
					task = strings.TrimSpace(sb.String())
				}
			}
			if task == "" {
				return fmt.Errorf("provide a task as argument or via stdin")
			}

			cfg, _ := LoadConfig()
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			router, err := newRouterFromConfig(cfg)
			if err != nil {
				return err
			}

			runner := meta.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
				wd, _ := os.Getwd()
				agentTools := []tools.Tool{
					tools.ReadTool(wd),
					tools.GlobTool(wd),
					tools.GrepTool(wd),
					tools.BashTool(30_000, nil),
				}
				return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
			})

			res, err := meta.Exec(cmd.Context(), task, repocontext.GatherRepoContext(), fetchSDKDocs(metaRefreshDocs), runner, cmd.OutOrStdout(), metaNoSynth)
			if err != nil {
				return err
			}
			if res.LogPath != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\nLogged to: %s\n", res.LogPath)
			}
			return nil
		},
	}
	metaCmd.Flags().BoolVar(&metaNoSynth, "no-synthesis", false, "Skip synthesis")
	metaCmd.Flags().BoolVar(&metaRefreshDocs, "refresh-docs", false, "Force re-fetch SDK docs")

	// diagnose subcommand
	var diagnoseService, diagnoseLogCmd string
	var diagnoseConfirm bool
	diagnoseCmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Diagnose a service failure from logs and system state",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			// Flags override config; config overrides defaults.
			service := diagnoseService
			if service == "" {
				service = cfg.Diagnose.Service
			}
			logCmd := diagnoseLogCmd
			if logCmd == "" {
				logCmd = cfg.Diagnose.LogCmd
			}

			// Data-sharing notice: diagnose sends shell command output to the
			// Anthropic API. Print once before the agent starts.
			effectiveLogCmd := logCmd
			if effectiveLogCmd == "" {
				effectiveLogCmd = diagnose.DefaultLogCmd()
			}
			fmt.Fprintf(os.Stderr, "Note: diagnose executes shell commands (e.g. %q) and sends their\n", effectiveLogCmd)
			fmt.Fprintf(os.Stderr, "output to the Anthropic API for analysis. Use --log-cmd to restrict scope.\n")

			router, err := newRouterFromConfig(cfg)
			if err != nil {
				return err
			}
			var confirmFn func(string) bool
			if diagnoseConfirm {
				scanner := bufio.NewScanner(os.Stdin)
				confirmFn = func(c string) bool {
					fmt.Fprintf(os.Stderr, "\nBashTool wants to run: %s\nAllow? [y/N] ", c)
					if !scanner.Scan() {
						return false
					}
					return strings.TrimSpace(strings.ToLower(scanner.Text())) == "y"
				}
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("diagnose", map[string]string{"service": service})
			start := time.Now()

			result, err := diagnose.Run(cmd.Context(), diagnose.Config{
				Service: service,
				LogCmd:  logCmd,
				Runner: diagnose.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					wd, _ := os.Getwd()
					agentTools := []tools.Tool{
						tools.ReadTool(wd),
						tools.GlobTool(wd),
						tools.GrepTool(wd),
						tools.BashTool(30_000, confirmFn),
					}
					return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
				}),
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "diagnose", map[string]string{"service": service}, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "diagnose", result, map[string]string{"service": service})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	diagnoseCmd.Flags().StringVar(&diagnoseService, "service", "", "Service/component to focus on")
	diagnoseCmd.Flags().StringVar(&diagnoseLogCmd, "log-cmd", "", fmt.Sprintf("Shell command to fetch logs (default: %s)", diagnose.DefaultLogCmd()))
	diagnoseCmd.Flags().BoolVar(&diagnoseConfirm, "confirm", false, "Prompt before each shell command the agent wants to run")

	// standup subcommand
	var standupSince string
	var standupRepos []string
	var standupParallel bool
	standupCmd := &cobra.Command{
		Use:   "standup",
		Short: "Summarize recent work as a standup update",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			since, err := time.ParseDuration(standupSince)
			if err != nil {
				return fmt.Errorf("invalid --since %q: %w", standupSince, err)
			}

			repos := standupRepos
			if len(repos) == 0 {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				repos = []string{wd}
			}

			router, err := newRouterFromConfig(cfg)
			if err != nil {
				return err
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("standup", map[string]string{"since": standupSince})
			start := time.Now()

			result, err := standup.Run(cmd.Context(), standup.Config{
				Repos: repos,
				Since: since,
				Runner: standup.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return router.For(providers.TierBalanced).Run(ctx, prompt, ts)
				}),
				Parallel: standupParallel,
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "standup", map[string]string{"since": standupSince}, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "standup", result, map[string]string{
				"since": standupSince,
				"repos": strings.Join(repos, ","),
			})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	standupCmd.Flags().StringVar(&standupSince, "since", "24h", "Time window (Go duration, e.g. 24h, 8h)")
	standupCmd.Flags().StringArrayVar(&standupRepos, "repo", nil, "Repo paths to include (repeatable, defaults to cwd)")
	standupCmd.Flags().BoolVar(&standupParallel, "parallel", false, "Summarize repos in parallel then synthesize")

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
		newChainCmd(nil, nil),
		newReplCmd(),
	)
	if err := root.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}
