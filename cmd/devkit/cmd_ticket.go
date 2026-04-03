package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	devlog "github.com/89jobrien/devkit/internal/log"
	"github.com/89jobrien/devkit/internal/providers"
	"github.com/89jobrien/devkit/internal/ticket"
	"github.com/spf13/cobra"
)

// newTicketCmd returns the ticket cobra command using the provided runner.
func newTicketCmd(runner ticket.Runner) *cobra.Command {
	var from string
	cmd := &cobra.Command{
		Use:   "ticket [description]",
		Short: "Generate a structured issue ticket",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := runner
			if r == nil {
				cfg, err := LoadConfig()
				if err != nil {
					return err
				}
				if cfg.Project.Name != "" {
					os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
				}
				router, err := newRouterFromConfig(cfg)
				if err != nil {
					return err
				}
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("ticket: cannot determine working directory: %w", err)
				}
				agentTools := agentToolsAt(wd)
				r = ticket.RunnerFunc(func(ctx context.Context, p string, ts []string) (string, error) {
					return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, p, ts)
				})
			}

			var prompt string
			inputSource := "arg"
			if len(args) > 0 {
				prompt = args[0]
			} else {
				info, _ := os.Stdin.Stat()
				if info.Mode()&os.ModeCharDevice == 0 {
					inputSource = "stdin"
					raw, readErr := io.ReadAll(os.Stdin)
					if readErr != nil && readErr != io.EOF {
						return fmt.Errorf("ticket: error reading stdin: %w", readErr)
					}
					prompt = strings.TrimSpace(string(raw))
				}
			}

			ticketPath := from
			if from != "" && prompt == "" {
				inputSource = "file"
				content, err := os.ReadFile(from)
				if err != nil {
					return fmt.Errorf("ticket: cannot read %s: %w", from, err)
				}
				prompt = fmt.Sprintf("Find TODOs, FIXMEs, and actionable issues in the following source file and generate a ticket for the most important one.\n\nFile: %s\n\n%s", from, string(content))
			}

			if strings.TrimSpace(prompt) == "" {
				return fmt.Errorf("ticket: provide a description as argument, via --from <file>, or via stdin")
			}

			logMeta := map[string]string{"from": ticketPath, "input": inputSource}
			sha := devlog.GitShortSHA()
			id := devlog.Start("ticket", logMeta)
			start := time.Now()

			result, err := ticket.Run(cmd.Context(), ticket.Config{
				Prompt: prompt,
				Path:   ticketPath,
				Runner: r,
			})
			if err != nil {
				return err
			}

			logResult(cmd.OutOrStdout(), "ticket", sha, logMeta, result, id, start)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Source file to extract TODOs/FIXMEs from")
	return cmd
}
