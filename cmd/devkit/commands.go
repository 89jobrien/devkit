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

	"github.com/89jobrien/devkit/internal/adr"
	"github.com/89jobrien/devkit/internal/baml"
	"github.com/89jobrien/devkit/internal/changelog"
	"github.com/89jobrien/devkit/internal/docgen"
	"github.com/89jobrien/devkit/internal/explain"
	"github.com/89jobrien/devkit/internal/incident"
	devlog "github.com/89jobrien/devkit/internal/log"
	"github.com/89jobrien/devkit/internal/lint"
	"github.com/89jobrien/devkit/internal/logpattern"
	"github.com/89jobrien/devkit/internal/migrate"
	"github.com/89jobrien/devkit/internal/pr"
	"github.com/89jobrien/devkit/internal/profile"
	"github.com/89jobrien/devkit/internal/providers"
	"github.com/89jobrien/devkit/internal/scaffold"
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
				base, err := buildTierRunner(providers.TierBalanced)
				if err != nil {
					return err
				}
				r = changelog.RunnerFunc(base.Run)
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

			logResult(cmd.OutOrStdout(), "changelog", sha, logMeta, result, id, start)
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
				base, err := buildTierRunner(providers.TierBalanced)
				if err != nil {
					return err
				}
				r = lint.RunnerFunc(base.Run)
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

			logResult(cmd.OutOrStdout(), "lint", sha, logMeta, result, id, start)
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

			logResult(cmd.OutOrStdout(), "explain", sha, logMeta, result, id, start)
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

			logResult(cmd.OutOrStdout(), "test-gen", sha, logMeta, result, id, start)
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

			logResult(cmd.OutOrStdout(), "ticket", sha, logMeta, result, id, start)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Source file to extract TODOs/FIXMEs from")
	return cmd
}

// newAdrCmd returns the adr cobra command using the provided runner.
func newAdrCmd(runner adr.Runner) *cobra.Command {
	var contextText string
	cmd := &cobra.Command{
		Use:   "adr <title>",
		Short: "Draft an Architecture Decision Record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				base, err := buildTierRunner(providers.TierFast)
				if err != nil {
					return err
				}
				r = adr.RunnerFunc(base.Run)
			}

			ctx := contextText
			if ctx == "" {
				info, _ := os.Stdin.Stat()
				if info.Mode()&os.ModeCharDevice == 0 {
					raw, readErr := io.ReadAll(os.Stdin)
					if readErr != nil && readErr != io.EOF {
						return fmt.Errorf("adr: error reading stdin: %w", readErr)
					}
					ctx = strings.TrimSpace(string(raw))
				}
			}

			logMeta := map[string]string{"title": args[0]}
			sha := devlog.GitShortSHA()
			id := devlog.Start("adr", logMeta)
			start := time.Now()

			result, err := adr.Run(cmd.Context(), adr.Config{
				Title:   args[0],
				Context: ctx,
				Runner:  r,
			})
			if err != nil {
				return fmt.Errorf("adr: %w", err)
			}

			logResult(cmd.OutOrStdout(), "adr", sha, logMeta, result, id, start)
			return nil
		},
	}
	cmd.Flags().StringVar(&contextText, "context", "", "Problem statement or context (or pipe via stdin)")
	return cmd
}

// newDocgenCmd returns the docgen cobra command using the provided runner.
func newDocgenCmd(runner docgen.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docgen <file>",
		Short: "Generate package-level Go docs from code",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				base, err := buildTierRunner(providers.TierFast)
				if err != nil {
					return err
				}
				r = docgen.RunnerFunc(base.Run)
			}

			filePath := args[0]
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("docgen: cannot read %s: %w", filePath, err)
			}

			logMeta := map[string]string{"file": filePath}
			sha := devlog.GitShortSHA()
			id := devlog.Start("docgen", logMeta)
			start := time.Now()

			result, err := docgen.Run(cmd.Context(), docgen.Config{
				File:   string(content),
				Path:   filePath,
				Runner: r,
			})
			if err != nil {
				return fmt.Errorf("docgen: %w", err)
			}

			logResult(cmd.OutOrStdout(), "docgen", sha, logMeta, result, id, start)
			return nil
		},
	}
	return cmd
}

// newMigrateCmd returns the migrate cobra command using the provided runner.
func newMigrateCmd(runner migrate.Runner) *cobra.Command {
	var oldSig, newSig string
	cmd := &cobra.Command{
		Use:   "migrate <file>",
		Short: "Analyze a breaking API change and suggest callsite updates",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				base, err := buildTierRunner(providers.TierBalanced)
				if err != nil {
					return err
				}
				r = migrate.RunnerFunc(base.Run)
			}

			if oldSig == "" || newSig == "" {
				return fmt.Errorf("migrate: --old and --new are required")
			}

			filePath := args[0]
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("migrate: cannot read %s: %w", filePath, err)
			}

			logMeta := map[string]string{"file": filePath}
			sha := devlog.GitShortSHA()
			id := devlog.Start("migrate", logMeta)
			start := time.Now()

			result, err := migrate.Run(cmd.Context(), migrate.Config{
				Old:    oldSig,
				New:    newSig,
				Code:   string(content),
				Path:   filePath,
				Runner: r,
			})
			if err != nil {
				return fmt.Errorf("migrate: %w", err)
			}

			logResult(cmd.OutOrStdout(), "migrate", sha, logMeta, result, id, start)
			return nil
		},
	}
	cmd.Flags().StringVar(&oldSig, "old", "", "Old API signature or description (required)")
	cmd.Flags().StringVar(&newSig, "new", "", "New API signature or description (required)")
	return cmd
}

// newScaffoldCmd returns the scaffold cobra command using the provided runner.
func newScaffoldCmd(runner scaffold.Runner) *cobra.Command {
	var purpose string
	cmd := &cobra.Command{
		Use:   "scaffold <package-name>",
		Short: "Generate boilerplate for a new Go package following hexagonal arch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				base, err := buildTierRunner(providers.TierFast)
				if err != nil {
					return err
				}
				r = scaffold.RunnerFunc(base.Run)
			}

			repoCtx := devlog.GatherRepoContext()

			logMeta := map[string]string{"name": args[0]}
			sha := devlog.GitShortSHA()
			id := devlog.Start("scaffold", logMeta)
			start := time.Now()

			result, err := scaffold.Run(cmd.Context(), scaffold.Config{
				Name:        args[0],
				Purpose:     purpose,
				RepoContext: repoCtx,
				Runner:      r,
			})
			if err != nil {
				return fmt.Errorf("scaffold: %w", err)
			}

			logResult(cmd.OutOrStdout(), "scaffold", sha, logMeta, result, id, start)
			return nil
		},
	}
	cmd.Flags().StringVar(&purpose, "purpose", "", "One-sentence description of the package purpose")
	return cmd
}

// newLogPatternCmd returns the log-pattern cobra command using the provided runner.
func newLogPatternCmd(runner logpattern.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log-pattern [file]",
		Short: "Find recurring error patterns across a log file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				base, err := buildTierRunner(providers.TierFast)
				if err != nil {
					return err
				}
				r = logpattern.RunnerFunc(base.Run)
			}

			var logs string
			if len(args) == 1 {
				content, err := os.ReadFile(args[0])
				if err != nil {
					return fmt.Errorf("log-pattern: cannot read %s: %w", args[0], err)
				}
				logs = string(content)
			} else {
				info, _ := os.Stdin.Stat()
				if info.Mode()&os.ModeCharDevice != 0 {
					return fmt.Errorf("log-pattern: provide a file argument or pipe input via stdin")
				}
				raw, readErr := io.ReadAll(os.Stdin)
				if readErr != nil && readErr != io.EOF {
					return fmt.Errorf("log-pattern: error reading stdin: %w", readErr)
				}
				logs = string(raw)
			}

			logMeta := map[string]string{}
			if len(args) == 1 {
				logMeta["file"] = args[0]
			}
			sha := devlog.GitShortSHA()
			id := devlog.Start("log-pattern", logMeta)
			start := time.Now()

			result, err := logpattern.Run(cmd.Context(), logpattern.Config{
				Logs:   logs,
				Runner: r,
			})
			if err != nil {
				return fmt.Errorf("log-pattern: %w", err)
			}

			logResult(cmd.OutOrStdout(), "log-pattern", sha, logMeta, result, id, start)
			return nil
		},
	}
	return cmd
}

// newIncidentCmd returns the incident cobra command using the provided runner.
func newIncidentCmd(runner incident.Runner) *cobra.Command {
	var description, logsFile string
	cmd := &cobra.Command{
		Use:   "incident",
		Short: "Generate a structured incident report from a description",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				base, err := buildTierRunner(providers.TierBalanced)
				if err != nil {
					return err
				}
				r = incident.RunnerFunc(base.Run)
			}

			if description == "" {
				return fmt.Errorf("incident: --description is required")
			}

			var logs string
			if logsFile != "" {
				content, err := os.ReadFile(logsFile)
				if err != nil {
					return fmt.Errorf("incident: cannot read %s: %w", logsFile, err)
				}
				logs = string(content)
			}

			logMeta := map[string]string{"logs": logsFile}
			sha := devlog.GitShortSHA()
			id := devlog.Start("incident", logMeta)
			start := time.Now()

			result, err := incident.Run(cmd.Context(), incident.Config{
				Description: description,
				Logs:        logs,
				Runner:      r,
			})
			if err != nil {
				return fmt.Errorf("incident: %w", err)
			}

			logResult(cmd.OutOrStdout(), "incident", sha, logMeta, result, id, start)
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "Incident description (required)")
	cmd.Flags().StringVar(&logsFile, "logs", "", "Optional log file to include")
	return cmd
}

// newProfileCmd returns the profile cobra command using the provided runner.
func newProfileCmd(runner profile.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile [file]",
		Short: "Analyze pprof or benchmark output with LLM commentary",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				base, err := buildTierRunner(providers.TierBalanced)
				if err != nil {
					return err
				}
				r = profile.RunnerFunc(base.Run)
			}

			var input string
			if len(args) == 1 {
				content, err := os.ReadFile(args[0])
				if err != nil {
					return fmt.Errorf("profile: cannot read %s: %w", args[0], err)
				}
				input = string(content)
			} else {
				info, _ := os.Stdin.Stat()
				if info.Mode()&os.ModeCharDevice != 0 {
					return fmt.Errorf("profile: provide a file argument or pipe input via stdin")
				}
				raw, readErr := io.ReadAll(os.Stdin)
				if readErr != nil && readErr != io.EOF {
					return fmt.Errorf("profile: error reading stdin: %w", readErr)
				}
				input = string(raw)
			}

			logMeta := map[string]string{}
			if len(args) == 1 {
				logMeta["file"] = args[0]
			}
			sha := devlog.GitShortSHA()
			id := devlog.Start("profile", logMeta)
			start := time.Now()

			result, err := profile.Run(cmd.Context(), profile.Config{
				Input:  input,
				Runner: r,
			})
			if err != nil {
				return fmt.Errorf("profile: %w", err)
			}

			logResult(cmd.OutOrStdout(), "profile", sha, logMeta, result, id, start)
			return nil
		},
	}
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

			logResult(cmd.OutOrStdout(), "pr", sha, logMeta, result, id, start)
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Base branch (default: auto-detect from GitHub)")
	return cmd
}
