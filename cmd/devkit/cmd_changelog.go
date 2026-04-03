package main

import (
	"time"

	"github.com/89jobrien/devkit/internal/changelog"
	devlog "github.com/89jobrien/devkit/internal/log"
	"github.com/89jobrien/devkit/internal/providers"
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
