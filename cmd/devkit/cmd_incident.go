package main

import (
	"fmt"
	"os"
	"time"

	"github.com/89jobrien/devkit/internal/baml"
	"github.com/89jobrien/devkit/internal/incident"
	devlog "github.com/89jobrien/devkit/internal/log"
	"github.com/89jobrien/devkit/internal/providers"
	"github.com/spf13/cobra"
)

// newIncidentCmd returns the incident cobra command using the provided runner.
func newIncidentCmd(runner incident.Runner) *cobra.Command {
	var description, logsFile string
	cmd := &cobra.Command{
		Use:   "incident",
		Short: "Generate a structured incident report from a description",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				base, err := buildTierRunner(providers.TierBalanced)
				if err != nil {
					return err
				}
				r = incident.RunnerFunc(base.Run)
			}

			if description == "" {
				return fmt.Errorf("incident: --description is required")
			}

			var logs string
			if logsFile != "" {
				content, err := os.ReadFile(logsFile)
				if err != nil {
					return fmt.Errorf("incident: cannot read %s: %w", logsFile, err)
				}
				logs = string(content)
			}

			logMeta := map[string]string{"logs": logsFile}
			sha := devlog.GitShortSHA()
			id := devlog.Start("incident", logMeta)
			start := time.Now()

			var (
				result string
				err    error
			)
			if runner != nil {
				result, err = incident.Run(cmd.Context(), incident.Config{
					Description: description,
					Logs:        logs,
					Runner:      runner,
				})
			} else {
				result, err = baml.RunIncident(cmd.Context(), description, logs)
			}
			if err != nil {
				return fmt.Errorf("incident: %w", err)
			}

			logResult(cmd.OutOrStdout(), "incident", sha, logMeta, result, id, start)
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "Incident description (required)")
	cmd.Flags().StringVar(&logsFile, "logs", "", "Optional log file to include")
	return cmd
}
