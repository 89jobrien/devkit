package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/89jobrien/devkit/internal/ai/baml"
	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/ops/profile"
	"github.com/89jobrien/devkit/internal/ai/providers"
	"github.com/spf13/cobra"
)

// newProfileCmd returns the profile cobra command using the provided runner.
func newProfileCmd(runner profile.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile [file]",
		Short: "Analyze pprof or benchmark output with LLM commentary",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				base, err := buildTierRunner(providers.TierBalanced)
				if err != nil {
					return err
				}
				r = profile.RunnerFunc(base.Run)
			}

			var input string
			if len(args) == 1 {
				content, err := os.ReadFile(args[0])
				if err != nil {
					return fmt.Errorf("profile: cannot read %s: %w", args[0], err)
				}
				input = string(content)
			} else {
				info, _ := os.Stdin.Stat()
				if info.Mode()&os.ModeCharDevice != 0 {
					return fmt.Errorf("profile: provide a file argument or pipe input via stdin")
				}
				raw, readErr := io.ReadAll(os.Stdin)
				if readErr != nil && readErr != io.EOF {
					return fmt.Errorf("profile: error reading stdin: %w", readErr)
				}
				input = string(raw)
			}

			logMeta := map[string]string{}
			if len(args) == 1 {
				logMeta["file"] = args[0]
			}
			sha := devlog.GitShortSHA()
			id := devlog.Start("profile", logMeta)
			start := time.Now()

			var (
				result string
				err    error
			)
			if runner != nil {
				result, err = profile.Run(cmd.Context(), profile.Config{
					Input:  input,
					Runner: runner,
				})
			} else {
				result, err = baml.RunProfile(cmd.Context(), input)
			}
			if err != nil {
				return fmt.Errorf("profile: %w", err)
			}

			logResult(cmd.OutOrStdout(), "profile", sha, logMeta, result, id, start)
			return nil
		},
	}
	return cmd
}
