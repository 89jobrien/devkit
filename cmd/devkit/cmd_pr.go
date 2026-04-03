package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/89jobrien/devkit/internal/ai/baml"
	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/dev/pr"
	"github.com/89jobrien/devkit/internal/ai/providers"
	"github.com/spf13/cobra"
)

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
