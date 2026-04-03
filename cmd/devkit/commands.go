// cmd/devkit/commands.go — cobra subcommand constructors for the new AI subcommands.
// Each constructor accepts an injected runner so tests can stub out LLM calls
// without loading config or touching the network.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/89jobrien/devkit/internal/baml"
	"github.com/89jobrien/devkit/internal/changelog"
	"github.com/89jobrien/devkit/internal/explain"
	devlog "github.com/89jobrien/devkit/internal/log"
	"github.com/89jobrien/devkit/internal/lint"
	"github.com/89jobrien/devkit/internal/pr"
	"github.com/89jobrien/devkit/internal/providers"
	"github.com/89jobrien/devkit/internal/testgen"
	"github.com/89jobrien/devkit/internal/ticket"
	"github.com/spf13/cobra"
)

// newChangelogCmd returns the changelog cobra command using the provided runner.
// Pass nil to have the command build its own runner from config (production path).
func newChangelogCmd(runner changelog.Runner) *cobra.Command {
	var base, format string
	cmd := &cobra.Command{
		Use:   "changelog",
		Short: "Generate a changelog from git log",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
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
				r = changelog.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return router.For(providers.TierBalanced).Run(ctx, prompt, ts)
				})
			}

			resolvedBase := base
			if resolvedBase == "" {
				resolvedBase = resolveChangelogBase()
			}
			log := gitLog(resolvedBase)

			logMeta := map[string]string{"base": resolvedBase, "format": format}
			sha := devlog.GitShortSHA()
			id := devlog.Start("changelog", logMeta)
			start := time.Now()

			result, err := changelog.Run(cmd.Context(), changelog.Config{
				Log:    log,
				Format: format,
				Runner: r,
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "changelog", logMeta, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "changelog", result, logMeta)
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Base ref (default: most recent git tag, fallback: main)")
	cmd.Flags().StringVar(&format, "format", "conventional", "Output format: conventional or prose")
	return cmd
}

// newLintCmd returns the lint cobra command using the provided runner.
func newLintCmd(runner lint.Runner) *cobra.Command {
	var role string
	cmd := &cobra.Command{
		Use:   "lint <file>",
		Short: "Single-file AI lint review",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
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
				r = lint.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return router.For(providers.TierBalanced).Run(ctx, prompt, ts)
				})
			}

			filePath := args[0]
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("lint: cannot read %s: %w", filePath, err)
			}

			logMeta := map[string]string{"file": filePath, "role": role}
			sha := devlog.GitShortSHA()
			id := devlog.Start("lint", logMeta)
			start := time.Now()

			result, err := lint.Run(cmd.Context(), lint.Config{
				File:   string(content),
				Path:   filePath,
				Role:   role,
				Runner: r,
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "lint", logMeta, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "lint", result, logMeta)
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "strict-critic", "Council role: strict-critic, security-reviewer, performance-analyst")
	return cmd
}

// newExplainCmd returns the explain cobra command using the provided runner.
func newExplainCmd(runner explain.Runner) *cobra.Command {
	var base, symbol string
	cmd := &cobra.Command{
		Use:   "explain [file]",
		Short: "Explain a file or diff in plain English",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
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
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("explain: cannot determine working directory: %w", err)
				}
				agentTools := agentToolsAt(wd)
				r = explain.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
				})
			}

			var cfg explain.Config
			cfg.Runner = r

			if len(args) == 1 {
				content, err := os.ReadFile(args[0])
				if err != nil {
					return fmt.Errorf("explain: cannot read %s: %w", args[0], err)
				}
				cfg.File = string(content)
				cfg.Path = args[0]
				cfg.Symbol = symbol
			} else {
				resolvedBase := base
				if resolvedBase == "" {
					resolvedBase = resolveDiffBase()
				}
				if err := validateRef(resolvedBase); err != nil {
					return fmt.Errorf("explain: %w", err)
				}
				cfg.Diff = gitDiff(resolvedBase)
				cfg.Log = gitLog(resolvedBase)
				cfg.Stat = gitStat(resolvedBase)
				base = resolvedBase
			}

			logMeta := map[string]string{"path": cfg.Path, "base": base}
			sha := devlog.GitShortSHA()
			id := devlog.Start("explain", logMeta)
			start := time.Now()

			result, err := explain.Run(cmd.Context(), cfg)
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "explain", logMeta, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "explain", result, logMeta)
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Base ref for diff mode (omit to explain a file)")
	cmd.Flags().StringVar(&symbol, "symbol", "", "Function or type to focus on (file mode only)")
	return cmd
}

// newTestgenCmd returns the test-gen cobra command using the provided runner.
func newTestgenCmd(runner testgen.Runner) *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:   "test-gen [file]",
		Short: "Generate Go test stubs for a file or diff",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
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
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("test-gen: cannot determine working directory: %w", err)
				}
				agentTools := agentToolsAt(wd)
				r = testgen.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
				})
			}

			var tgCfg testgen.Config
			tgCfg.Runner = r

			if len(args) == 1 {
				content, err := os.ReadFile(args[0])
				if err != nil {
					return fmt.Errorf("test-gen: cannot read %s: %w", args[0], err)
				}
				tgCfg.File = string(content)
				tgCfg.Path = args[0]
			} else {
				resolvedBase := base
				if resolvedBase == "" {
					resolvedBase = resolveDiffBase()
				}
				if err := validateRef(resolvedBase); err != nil {
					return fmt.Errorf("test-gen: %w", err)
				}
				tgCfg.Diff = gitDiff(resolvedBase)
				tgCfg.Log = gitLog(resolvedBase)
				base = resolvedBase
			}

			logMeta := map[string]string{"path": tgCfg.Path, "base": base}
			sha := devlog.GitShortSHA()
			id := devlog.Start("test-gen", logMeta)
			start := time.Now()

			result, err := testgen.Run(cmd.Context(), tgCfg)
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "test-gen", logMeta, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "test-gen", result, logMeta)
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Base ref for diff mode (omit to generate tests for a file)")
	return cmd
}

// newTicketCmd returns the ticket cobra command using the provided runner.
func newTicketCmd(runner ticket.Runner) *cobra.Command {
	var from string
	cmd := &cobra.Command{
		Use:   "ticket [description]",
		Short: "Generate a structured issue ticket",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
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
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("ticket: cannot determine working directory: %w", err)
				}
				agentTools := agentToolsAt(wd)
				r = ticket.RunnerFunc(func(ctx context.Context, p string, ts []string) (string, error) {
					return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, p, ts)
				})
			}

			var prompt string
			inputSource := "arg"
			if len(args) > 0 {
				prompt = args[0]
			} else {
				info, _ := os.Stdin.Stat()
				if info.Mode()&os.ModeCharDevice == 0 {
					inputSource = "stdin"
					raw, readErr := io.ReadAll(os.Stdin)
					if readErr != nil && readErr != io.EOF {
						return fmt.Errorf("ticket: error reading stdin: %w", readErr)
					}
					prompt = strings.TrimSpace(string(raw))
				}
			}

			ticketPath := from
			if from != "" && prompt == "" {
				inputSource = "file"
				content, err := os.ReadFile(from)
				if err != nil {
					return fmt.Errorf("ticket: cannot read %s: %w", from, err)
				}
				prompt = fmt.Sprintf("Find TODOs, FIXMEs, and actionable issues in the following source file and generate a ticket for the most important one.\n\nFile: %s\n\n%s", from, string(content))
			}

			if strings.TrimSpace(prompt) == "" {
				return fmt.Errorf("ticket: provide a description as argument, via --from <file>, or via stdin")
			}

			logMeta := map[string]string{"from": ticketPath, "input": inputSource}
			sha := devlog.GitShortSHA()
			id := devlog.Start("ticket", logMeta)
			start := time.Now()

			result, err := ticket.Run(cmd.Context(), ticket.Config{
				Prompt: prompt,
				Path:   ticketPath,
				Runner: r,
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "ticket", logMeta, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "ticket", result, logMeta)
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Source file to extract TODOs/FIXMEs from")
	return cmd
}

// newPrCmd returns the pr cobra command using the provided runner.
func newPrCmd(runner pr.Runner) *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Draft a pull request description from branch diff",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				cfg, err := LoadConfig()
				if err != nil {
					return err
				}
				if cfg.Project.Name != "" {
					os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
				}
				if cfg.Providers.UseBAML {
					r = baml.New("pr", os.Stdout)
				} else {
					router, err := newRouterFromConfig(cfg)
					if err != nil {
						return err
					}
					r = pr.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
						return router.For(providers.TierBalanced).Run(ctx, prompt, ts)
					})
				}
			}

			resolvedBase := pr.ResolveBase(base)
			if err := validateRef(resolvedBase); err != nil {
				return fmt.Errorf("pr: %w", err)
			}
			fmt.Fprintf(os.Stderr, "devkit: generating PR description from base %q\n", resolvedBase)

			diff := gitDiff(resolvedBase)
			commitLog := gitLog(resolvedBase)
			stat := gitStat(resolvedBase)

			logMeta := map[string]string{"base": resolvedBase}
			sha := devlog.GitShortSHA()
			id := devlog.Start("pr", logMeta)
			start := time.Now()

			result, err := pr.Run(cmd.Context(), pr.Config{
				Base:   resolvedBase,
				Diff:   diff,
				Log:    commitLog,
				Stat:   stat,
				Runner: r,
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "pr", logMeta, result, time.Since(start))
			logPath, logErr := devlog.SaveCommitLog(sha, "pr", result, logMeta)
			if logErr != nil {
				fmt.Fprintf(os.Stderr, "devkit: warning: failed to save commit log: %v\n", logErr)
			} else {
				fmt.Printf("\nLogged to: %s\n", logPath)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Base branch (default: auto-detect from GitHub)")
	return cmd
}
