package main

import (
	"fmt"
	"os"
	"time"

	"github.com/89jobrien/devkit/internal/ai/baml"
	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/dev/migrate"
	"github.com/spf13/cobra"
)

// newMigrateCmd returns the migrate cobra command using the provided runner.
func newMigrateCmd(runner migrate.Runner) *cobra.Command {
	var oldSig, newSig string
	cmd := &cobra.Command{
		Use:   "migrate <file>",
		Short: "Analyze a breaking API change and suggest callsite updates",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if oldSig == "" || newSig == "" {
				return fmt.Errorf("migrate: --old and --new are required")
			}

			filePath := args[0]
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("migrate: cannot read %s: %w", filePath, err)
			}

			logMeta := map[string]string{"file": filePath}
			sha := devlog.GitShortSHA()
			id := devlog.Start("migrate", logMeta)
			start := time.Now()

			var result string
			if runner != nil {
				result, err = migrate.Run(cmd.Context(), migrate.Config{
					Old:    oldSig,
					New:    newSig,
					Code:   string(content),
					Path:   filePath,
					Runner: runner,
				})
			} else {
				result, err = baml.RunMigrate(cmd.Context(), oldSig, newSig, string(content), filePath)
			}
			if err != nil {
				return fmt.Errorf("migrate: %w", err)
			}

			logResult(cmd.OutOrStdout(), "migrate", sha, logMeta, result, id, start)
			return nil
		},
	}
	cmd.Flags().StringVar(&oldSig, "old", "", "Old API signature or description (required)")
	cmd.Flags().StringVar(&newSig, "new", "", "New API signature or description (required)")
	return cmd
}
