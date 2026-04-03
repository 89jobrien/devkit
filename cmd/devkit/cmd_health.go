package main

import (
	"context"
	"fmt"
	"time"

	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/ai/baml"
	"github.com/89jobrien/devkit/internal/ops/health"
	"github.com/spf13/cobra"
)

func newHealthCmd(runner health.Runner) *cobra.Command {
	var repo, format string
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Run repo health checks and emit a scored report",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				r = health.RunnerFunc(func(ctx context.Context, repoCtx, checks string) (string, error) {
					return baml.RunHealth(ctx, repoCtx, checks)
				})
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("health", map[string]string{"repo": repo, "format": format})
			start := time.Now()

			result, err := health.Run(cmd.Context(), health.Config{
				RepoPath: repo,
				Runner:   r,
			})
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), result)

			devlog.Complete(id, "health", map[string]string{"repo": repo, "format": format}, result, time.Since(start))
			_, _ = devlog.SaveCommitLog(sha, "health", result, map[string]string{"repo": repo})
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Repo path (default: cwd)")
	cmd.Flags().StringVar(&format, "format", "markdown", "Output format: markdown or json")
	return cmd
}
