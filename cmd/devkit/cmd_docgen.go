package main

import (
	"fmt"
	"os"
	"time"

	"github.com/89jobrien/devkit/internal/ai/baml"
	"github.com/89jobrien/devkit/internal/dev/docgen"
	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/spf13/cobra"
)

// newDocgenCmd returns the docgen cobra command using the provided runner.
func newDocgenCmd(runner docgen.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docgen <file>",
		Short: "Generate package-level Go docs from code",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("docgen: cannot read %s: %w", filePath, err)
			}

			logMeta := map[string]string{"file": filePath}
			sha := devlog.GitShortSHA()
			id := devlog.Start("docgen", logMeta)
			start := time.Now()

			var result string
			if runner != nil {
				result, err = docgen.Run(cmd.Context(), docgen.Config{
					File:   string(content),
					Path:   filePath,
					Runner: runner,
				})
			} else {
				result, err = baml.RunDocgen(cmd.Context(), string(content), filePath)
			}
			if err != nil {
				return fmt.Errorf("docgen: %w", err)
			}

			logResult(cmd.OutOrStdout(), "docgen", sha, logMeta, result, id, start)
			return nil
		},
	}
	return cmd
}
