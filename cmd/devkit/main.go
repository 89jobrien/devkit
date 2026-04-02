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

	"github.com/89jobrien/devkit/internal/baml"
	"github.com/89jobrien/devkit/internal/changelog"
	"github.com/89jobrien/devkit/internal/council"
	"github.com/89jobrien/devkit/internal/diagnose"
	"github.com/89jobrien/devkit/internal/explain"
	"github.com/89jobrien/devkit/internal/lint"
	devlog "github.com/89jobrien/devkit/internal/log"
	"github.com/89jobrien/devkit/internal/meta"
	"github.com/89jobrien/devkit/internal/pr"
	"github.com/89jobrien/devkit/internal/providers"
	"github.com/89jobrien/devkit/internal/review"
	"github.com/89jobrien/devkit/internal/standup"
	"github.com/89jobrien/devkit/internal/testgen"
	"github.com/89jobrien/devkit/internal/ticket"
	"github.com/89jobrien/devkit/internal/tools"
	"github.com/spf13/cobra"
)

func gitDiff(base string) string {
	out, err := exec.Command("git", "diff", base+"...HEAD").Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		out, _ = exec.Command("git", "diff", "HEAD").Output()
	}
	return string(out)
}

func gitLog(base string) string {
	out, _ := exec.Command("git", "log", base+"...HEAD", "--oneline").Output()
	return string(out)
}

func gitStat(base string) string {
	out, _ := exec.Command("git", "diff", base+"...HEAD", "--stat").Output()
	return string(out)
}

func resolveChangelogBase() string {
	out, err := exec.Command("git", "describe", "--tags", "--abbrev=0").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return "main"
	}
	return strings.TrimSpace(string(out))
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

			diff := gitDiff(councilBase)
			commits := gitLog(councilBase)
			stat := gitStat(councilBase)

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
			diff := gitDiff(reviewBase)
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

			repoContext := devlog.GatherRepoContext()
			sdkDocs := fetchSDKDocs(metaRefreshDocs)

			router, err := newRouterFromConfig(cfg)
			if err != nil {
				return err
			}

			sha := devlog.GitShortSHA()

			taskPreview := task
			if len(taskPreview) > 80 {
				taskPreview = taskPreview[:80]
			}

			id := devlog.Start("meta", map[string]string{"task": taskPreview})
			start := time.Now()

			result, err := meta.Run(cmd.Context(), task, repoContext, sdkDocs,
				meta.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					wd, _ := os.Getwd()
					agentTools := []tools.Tool{
						tools.ReadTool(wd),
						tools.GlobTool(wd),
						tools.GrepTool(wd),
						tools.BashTool(30_000, nil),
					}
					return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
				}))
			if err != nil {
				return err
			}

			var allOutput strings.Builder
			for name, out := range result.Outputs {
				fmt.Printf("\n---- %s ----\n%s\n", name, out)
				allOutput.WriteString(fmt.Sprintf("## %s\n%s\n\n", name, out))
			}
			if !metaNoSynth {
				fmt.Printf("\n---- SYNTHESIS ----\n%s\n", result.Summary)
				allOutput.WriteString(fmt.Sprintf("## Synthesis\n%s\n", result.Summary))
			}

			devlog.Complete(id, "meta", map[string]string{"task": taskPreview},
				allOutput.String(), time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "meta", allOutput.String(), map[string]string{"task": taskPreview})
			fmt.Printf("\nLogged to: %s\n", path)
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

	// pr subcommand
	var prBase string
	prCmd := &cobra.Command{
		Use:   "pr",
		Short: "Draft a pull request description from branch diff",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			base := pr.ResolveBase(prBase)
			diff := gitDiff(base)
			commitLog := gitLog(base)
			stat := gitStat(base)

			var runner pr.Runner
			if cfg.Providers.UseBAML {
				runner = baml.New("pr", os.Stdout)
			} else {
				router, err := newRouterFromConfig(cfg)
				if err != nil {
					return err
				}
				runner = pr.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return router.For(providers.TierBalanced).Run(ctx, prompt, ts)
				})
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("pr", map[string]string{"base": base})
			start := time.Now()

			result, err := pr.Run(cmd.Context(), pr.Config{
				Base:   base,
				Diff:   diff,
				Log:    commitLog,
				Stat:   stat,
				Runner: runner,
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "pr", map[string]string{"base": base}, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "pr", result, map[string]string{"base": base})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	prCmd.Flags().StringVar(&prBase, "base", "", "Base branch (default: auto-detect from GitHub)")

	// changelog subcommand
	var changelogBase, changelogFormat string
	changelogCmd := &cobra.Command{
		Use:   "changelog",
		Short: "Generate a changelog from git log",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			base := changelogBase
			if base == "" {
				base = resolveChangelogBase()
			}
			log := gitLog(base)

			router, err := newRouterFromConfig(cfg)
			if err != nil {
				return err
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("changelog", map[string]string{"base": base, "format": changelogFormat})
			start := time.Now()

			result, err := changelog.Run(cmd.Context(), changelog.Config{
				Log:    log,
				Format: changelogFormat,
				Runner: changelog.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return router.For(providers.TierBalanced).Run(ctx, prompt, ts)
				}),
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "changelog", map[string]string{"base": base, "format": changelogFormat}, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "changelog", result, map[string]string{"base": base, "format": changelogFormat})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	changelogCmd.Flags().StringVar(&changelogBase, "base", "", "Base ref (default: most recent git tag, fallback: main)")
	changelogCmd.Flags().StringVar(&changelogFormat, "format", "conventional", "Output format: conventional or prose")

	// lint subcommand
	var lintRole string
	lintCmd := &cobra.Command{
		Use:   "lint <file>",
		Short: "Single-file AI lint review",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			filePath := args[0]
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("lint: cannot read %s: %w", filePath, err)
			}

			router, err := newRouterFromConfig(cfg)
			if err != nil {
				return err
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("lint", map[string]string{"file": filePath, "role": lintRole})
			start := time.Now()

			result, err := lint.Run(cmd.Context(), lint.Config{
				File:   string(content),
				Path:   filePath,
				Role:   lintRole,
				Runner: lint.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return router.For(providers.TierBalanced).Run(ctx, prompt, ts)
				}),
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "lint", map[string]string{"file": filePath, "role": lintRole}, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "lint", result, map[string]string{"file": filePath, "role": lintRole})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	lintCmd.Flags().StringVar(&lintRole, "role", "strict-critic", "Council role: strict-critic, security-reviewer, performance-analyst")

	// explain subcommand
	var explainBase, explainSymbol string
	explainCmd := &cobra.Command{
		Use:   "explain [file]",
		Short: "Explain a file or diff in plain English",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			router, err := newRouterFromConfig(cfg)
			if err != nil {
				return err
			}

			wd, _ := os.Getwd()
			agentTools := []tools.Tool{
				tools.ReadTool(wd),
				tools.GlobTool(wd),
				tools.GrepTool(wd),
			}
			runner := explain.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
				return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
			})

			var explainCfg explain.Config
			explainCfg.Runner = runner

			if len(args) == 1 {
				content, err := os.ReadFile(args[0])
				if err != nil {
					return fmt.Errorf("explain: cannot read %s: %w", args[0], err)
				}
				explainCfg.File = string(content)
				explainCfg.Path = args[0]
				explainCfg.Symbol = explainSymbol
			} else {
				base := explainBase
				if base == "" {
					base = "main"
				}
				explainCfg.Diff = gitDiff(base)
				explainCfg.Log = gitLog(base)
				explainCfg.Stat = gitStat(base)
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("explain", map[string]string{"path": explainCfg.Path, "base": explainBase})
			start := time.Now()

			result, err := explain.Run(cmd.Context(), explainCfg)
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "explain", map[string]string{"path": explainCfg.Path}, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "explain", result, map[string]string{"path": explainCfg.Path})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	explainCmd.Flags().StringVar(&explainBase, "base", "", "Base ref for diff mode (omit to explain a file)")
	explainCmd.Flags().StringVar(&explainSymbol, "symbol", "", "Function or type to focus on (file mode only)")

	// test-gen subcommand
	var testgenBase string
	testgenCmd := &cobra.Command{
		Use:   "test-gen [file]",
		Short: "Generate Go test stubs for a file or diff",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			router, err := newRouterFromConfig(cfg)
			if err != nil {
				return err
			}

			wd, _ := os.Getwd()
			agentTools := []tools.Tool{
				tools.ReadTool(wd),
				tools.GlobTool(wd),
				tools.GrepTool(wd),
			}
			runner := testgen.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
				return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
			})

			var tgCfg testgen.Config
			tgCfg.Runner = runner

			if len(args) == 1 {
				content, err := os.ReadFile(args[0])
				if err != nil {
					return fmt.Errorf("test-gen: cannot read %s: %w", args[0], err)
				}
				tgCfg.File = string(content)
				tgCfg.Path = args[0]
			} else {
				base := testgenBase
				if base == "" {
					base = "main"
				}
				tgCfg.Diff = gitDiff(base)
				tgCfg.Log = gitLog(base)
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("test-gen", map[string]string{"path": tgCfg.Path, "base": testgenBase})
			start := time.Now()

			result, err := testgen.Run(cmd.Context(), tgCfg)
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "test-gen", map[string]string{"path": tgCfg.Path}, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "test-gen", result, map[string]string{"path": tgCfg.Path})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	testgenCmd.Flags().StringVar(&testgenBase, "base", "", "Base ref for diff mode (omit to generate tests for a file)")

	// ticket subcommand
	var ticketFrom string
	ticketCmd := &cobra.Command{
		Use:   "ticket [description]",
		Short: "Generate a structured issue ticket",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			var prompt string
			if len(args) > 0 {
				prompt = args[0]
			} else {
				info, _ := os.Stdin.Stat()
				if info.Mode()&os.ModeCharDevice == 0 {
					var sb strings.Builder
					buf := make([]byte, 4096)
					for {
						n, readErr := os.Stdin.Read(buf)
						sb.Write(buf[:n])
						if readErr != nil {
							break
						}
					}
					prompt = strings.TrimSpace(sb.String())
				}
			}

			// In code-context mode, read file and prepend found TODOs/FIXMEs to prompt.
			ticketPath := ticketFrom
			if ticketFrom != "" && prompt == "" {
				content, err := os.ReadFile(ticketFrom)
				if err != nil {
					return fmt.Errorf("ticket: cannot read %s: %w", ticketFrom, err)
				}
				prompt = fmt.Sprintf("Find TODOs, FIXMEs, and actionable issues in the following source file and generate a ticket for the most important one.\n\nFile: %s\n\n%s", ticketFrom, string(content))
			}

			if strings.TrimSpace(prompt) == "" {
				return fmt.Errorf("ticket: provide a description as argument, via --from <file>, or via stdin")
			}

			router, err := newRouterFromConfig(cfg)
			if err != nil {
				return err
			}

			wd, _ := os.Getwd()
			agentTools := []tools.Tool{
				tools.ReadTool(wd),
				tools.GlobTool(wd),
				tools.GrepTool(wd),
			}
			runner := ticket.RunnerFunc(func(ctx context.Context, p string, ts []string) (string, error) {
				return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, p, ts)
			})

			sha := devlog.GitShortSHA()
			id := devlog.Start("ticket", map[string]string{"from": ticketPath})
			start := time.Now()

			result, err := ticket.Run(cmd.Context(), ticket.Config{
				Prompt: prompt,
				Path:   ticketPath,
				Runner: runner,
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "ticket", map[string]string{"from": ticketPath}, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "ticket", result, map[string]string{"from": ticketPath})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	ticketCmd.Flags().StringVar(&ticketFrom, "from", "", "Source file to extract TODOs/FIXMEs from")

	root.AddCommand(councilCmd, reviewCmd, metaCmd, diagnoseCmd, standupCmd, prCmd, changelogCmd, lintCmd, explainCmd, testgenCmd, ticketCmd)
	if err := root.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}
