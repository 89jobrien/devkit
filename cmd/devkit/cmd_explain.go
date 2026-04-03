package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/89jobrien/devkit/internal/dev/explain"
	devgit "github.com/89jobrien/devkit/internal/infra/git"
	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/ai/providers"
	"github.com/spf13/cobra"
)

// newExplainCmd returns the explain cobra command using the provided runner.
func newExplainCmd(runner explain.Runner, resolver devgit.RangeResolver) *cobra.Command {
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
				if resolver == nil {
					resolver = devgit.ExecRangeResolver{}
				}
				rangeResult, err := resolver.ResolveRange(resolvedBase)
				if err != nil {
					return fmt.Errorf("explain: resolve git range: %w", err)
				}
				cfg.Diff, err = devgit.Diff(rangeResult)
				if err != nil {
					return fmt.Errorf("explain: git diff: %w", err)
				}
				cfg.Log, err = devgit.Log(rangeResult)
				if err != nil {
					return fmt.Errorf("explain: git log: %w", err)
				}
				cfg.Stat, err = devgit.Stat(rangeResult)
				if err != nil {
					return fmt.Errorf("explain: git stat: %w", err)
				}
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
