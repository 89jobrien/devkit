package main

import (
	"context"
	"fmt"
	"os"
	"time"

	devgit "github.com/89jobrien/devkit/internal/infra/git"
	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/ai/providers"
	"github.com/89jobrien/devkit/internal/dev/testgen"
	"github.com/spf13/cobra"
)

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
				rangeResult, err := devgit.ExecRangeResolver{}.ResolveRange(resolvedBase)
				if err != nil {
					return fmt.Errorf("test-gen: resolve git range: %w", err)
				}
				tgCfg.Diff, err = devgit.Diff(rangeResult)
				if err != nil {
					return fmt.Errorf("test-gen: git diff: %w", err)
				}
				tgCfg.Log, err = devgit.Log(rangeResult)
				if err != nil {
					return fmt.Errorf("test-gen: git log: %w", err)
				}
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
