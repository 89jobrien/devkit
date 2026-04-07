package main

import (
	"fmt"
	"time"

	"github.com/89jobrien/devkit/internal/ai/baml"
	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/dev/scaffold"
	"github.com/89jobrien/devkit/internal/repocontext"
	"github.com/spf13/cobra"
)

// newScaffoldCmd returns the scaffold cobra command using the provided runner.
func newScaffoldCmd(runner scaffold.Runner) *cobra.Command {
	var purpose string
	cmd := &cobra.Command{
		Use:   "scaffold <package-name>",
		Short: "Generate boilerplate for a new Go package following hexagonal arch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoCtx := repocontext.GatherRepoContext()

			logMeta := map[string]string{"name": args[0]}
			sha := devlog.GitShortSHA()
			id := devlog.Start("scaffold", logMeta)
			start := time.Now()

			var result string
			var err error
			if runner != nil {
				result, err = scaffold.Run(cmd.Context(), scaffold.Config{
					Name:        args[0],
					Purpose:     purpose,
					RepoContext: repoCtx,
					Runner:      runner,
				})
			} else {
				result, err = baml.RunScaffold(cmd.Context(), args[0], purpose, repoCtx)
			}
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
