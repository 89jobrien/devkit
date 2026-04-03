package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/89jobrien/devkit/internal/baml"
	devlog "github.com/89jobrien/devkit/internal/log"
	"github.com/89jobrien/devkit/internal/logpattern"
	"github.com/89jobrien/devkit/internal/providers"
	"github.com/spf13/cobra"
)

// newLogPatternCmd returns the log-pattern cobra command using the provided runner.
func newLogPatternCmd(runner logpattern.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log-pattern [file]",
		Short: "Find recurring error patterns across a log file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				base, err := buildTierRunner(providers.TierFast)
				if err != nil {
					return err
				}
				r = logpattern.RunnerFunc(base.Run)
			}

			var logs string
			if len(args) == 1 {
				content, err := os.ReadFile(args[0])
				if err != nil {
					return fmt.Errorf("log-pattern: cannot read %s: %w", args[0], err)
				}
				logs = string(content)
			} else {
				info, _ := os.Stdin.Stat()
				if info.Mode()&os.ModeCharDevice != 0 {
					return fmt.Errorf("log-pattern: provide a file argument or pipe input via stdin")
				}
				raw, readErr := io.ReadAll(os.Stdin)
				if readErr != nil && readErr != io.EOF {
					return fmt.Errorf("log-pattern: error reading stdin: %w", readErr)
				}
				logs = string(raw)
			}

			logMeta := map[string]string{}
			if len(args) == 1 {
				logMeta["file"] = args[0]
			}
			sha := devlog.GitShortSHA()
			id := devlog.Start("log-pattern", logMeta)
			start := time.Now()

			var result string
			var err error
			if runner != nil {
				result, err = logpattern.Run(cmd.Context(), logpattern.Config{
					Logs:   logs,
					Runner: runner,
				})
			} else {
				result, err = baml.RunLogPattern(cmd.Context(), logs)
			}
			if err != nil {
				return fmt.Errorf("log-pattern: %w", err)
			}

			logResult(cmd.OutOrStdout(), "log-pattern", sha, logMeta, result, id, start)
			return nil
		},
	}
	return cmd
}
