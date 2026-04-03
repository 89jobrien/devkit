package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/89jobrien/devkit/internal/dev/adr"
	"github.com/89jobrien/devkit/internal/ai/baml"
	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/spf13/cobra"
)

// newAdrCmd returns the adr cobra command using the provided runner.
func newAdrCmd(runner adr.Runner) *cobra.Command {
	var contextText string
	cmd := &cobra.Command{
		Use:   "adr <title>",
		Short: "Draft an Architecture Decision Record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxInput := contextText
			if ctxInput == "" {
				info, _ := os.Stdin.Stat()
				if info.Mode()&os.ModeCharDevice == 0 {
					raw, readErr := io.ReadAll(os.Stdin)
					if readErr != nil && readErr != io.EOF {
						return fmt.Errorf("adr: error reading stdin: %w", readErr)
					}
					ctxInput = strings.TrimSpace(string(raw))
				}
			}

			logMeta := map[string]string{"title": args[0]}
			sha := devlog.GitShortSHA()
			id := devlog.Start("adr", logMeta)
			start := time.Now()

			var result string
			var err error
			if runner != nil {
				result, err = adr.Run(cmd.Context(), adr.Config{
					Title:   args[0],
					Context: ctxInput,
					Runner:  runner,
				})
			} else {
				result, err = baml.RunADR(cmd.Context(), args[0], ctxInput)
			}
			if err != nil {
				return fmt.Errorf("adr: %w", err)
			}

			logResult(cmd.OutOrStdout(), "adr", sha, logMeta, result, id, start)
			return nil
		},
	}
	cmd.Flags().StringVar(&contextText, "context", "", "Problem statement or context (or pipe via stdin)")
	return cmd
}
