package main

import (
	"fmt"
	"strings"
	"time"

	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/ai/providers"
	"github.com/89jobrien/devkit/internal/ops/automate"
	"github.com/spf13/cobra"
)

func newAutomateCmd(runner automate.Runner) *cobra.Command {
	var repo, tasks string
	cmd := &cobra.Command{
		Use:   "automate",
		Short: "Run routine maintenance tasks (changelog, standup, tickets)",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				base, err := buildTierRunner(providers.TierBalanced)
				if err != nil {
					return err
				}
				r = automate.RunnerFunc(base.Run)
			}

			taskList := strings.Split(tasks, ",")

			sha := devlog.GitShortSHA()
			id := devlog.Start("automate", map[string]string{"tasks": tasks, "repo": repo})
			start := time.Now()

			result, err := automate.Run(cmd.Context(), automate.Config{
				Tasks:    taskList,
				RepoPath: repo,
				Runner:   r,
			})
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), result)

			devlog.Complete(id, "automate", map[string]string{"tasks": tasks}, result, time.Since(start))
			_, _ = devlog.SaveCommitLog(sha, "automate", result, map[string]string{"tasks": tasks})
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Repo path (default: cwd)")
	cmd.Flags().StringVar(&tasks, "tasks", "changelog,standup,tickets", "Comma-separated tasks to run")
	return cmd
}
