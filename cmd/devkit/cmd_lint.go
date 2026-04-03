package main

import (
	"fmt"
	"os"
	"time"

	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/dev/lint"
	"github.com/89jobrien/devkit/internal/ai/providers"
	"github.com/spf13/cobra"
)

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
