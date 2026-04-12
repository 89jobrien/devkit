package main

import (
	"fmt"
	"time"

	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/ai/council"
	"github.com/89jobrien/devkit/internal/ai/providers"
	"github.com/89jobrien/devkit/internal/dev/reporeview"
	"github.com/spf13/cobra"
)

func newRepoReviewCmd(runner council.Runner) *cobra.Command {
	var repo, format string
	cmd := &cobra.Command{
		Use:   "repo-review",
		Short: "Council-style review of overall repo health",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				base, err := buildTierRunner(providers.TierBalanced)
				if err != nil {
					return err
				}
				r = base
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("repo-review", map[string]string{"repo": repo, "format": format})
			start := time.Now()

			result, err := reporeview.Run(cmd.Context(), reporeview.Config{
				RepoPath: repo,
				Runner:   r,
				Format:   format,
			})
			if result != "" {
				fmt.Fprintln(cmd.OutOrStdout(), result)
			}

			// Log completion regardless of error so the dev log reflects the actual
			// outcome — failed runs previously appeared as hung/incomplete.
			devlog.Complete(id, "repo-review", map[string]string{"repo": repo, "format": format}, result, time.Since(start))
			_, _ = devlog.SaveCommitLog(sha, "repo-review", result, map[string]string{"repo": repo})
			return err
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Repo path (default: cwd)")
	cmd.Flags().StringVar(&format, "format", "markdown", "Output format: markdown or json")
	return cmd
}
