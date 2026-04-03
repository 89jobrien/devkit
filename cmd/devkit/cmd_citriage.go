package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/ai/baml"
	"github.com/89jobrien/devkit/internal/ops/citriage"
	"github.com/spf13/cobra"
)

func newCITriageCmd(runner citriage.Runner) *cobra.Command {
	var repo, runID string
	var fromStdin bool
	cmd := &cobra.Command{
		Use:   "ci-triage",
		Short: "Diagnose CI failure logs with LLM triage",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				r = citriage.RunnerFunc(func(ctx context.Context, log, repoCtx string) (string, error) {
					return baml.RunCITriage(ctx, log, repoCtx)
				})
			}

			var preloadedLog string
			if fromStdin {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("reading stdin: %w", err)
				}
				preloadedLog = string(data)
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("ci-triage", map[string]string{"run": runID, "stdin": fmt.Sprintf("%v", fromStdin)})
			start := time.Now()

			result, err := citriage.Run(cmd.Context(), citriage.Config{
				RepoPath: repo,
				RunID:    runID,
				Log:      preloadedLog,
				Runner:   r,
			})
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), result)

			devlog.Complete(id, "ci-triage", map[string]string{"run": runID}, result, time.Since(start))
			_, _ = devlog.SaveCommitLog(sha, "ci-triage", result, map[string]string{"run": runID})
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Repo path (default: cwd)")
	cmd.Flags().StringVar(&runID, "run", "", "GitHub Actions run ID to fetch")
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "Read failure log from stdin")
	return cmd
}
