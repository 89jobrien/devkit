// cmd/devkit/cmd_repl.go
package main

import (
	"fmt"
	"os"

	"github.com/89jobrien/devkit/internal/chain"
	"github.com/89jobrien/devkit/internal/repl"
	"github.com/spf13/cobra"
)

func newReplCmd() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:   "repl",
		Short: "Start an interactive REPL session with persistent auth and context",
		RunE: func(cmd *cobra.Command, args []string) error {
			antKey := os.Getenv("ANTHROPIC_API_KEY")
			oaiKey := os.Getenv("OPENAI_API_KEY")
			gemKey := os.Getenv("GEMINI_API_KEY")

			if antKey == "" && oaiKey == "" {
				return fmt.Errorf("repl: ANTHROPIC_API_KEY or OPENAI_API_KEY required")
			}
			if oaiKey == "" {
				return fmt.Errorf("repl: OPENAI_API_KEY required for synthesis stage")
			}

			runners := chain.BuildStageRunners(chain.StageWiringConfig{
				RepoPath:     repo,
				AnthropicKey: antKey,
				OpenAIKey:    oaiKey,
				GeminiKey:    gemKey,
			})
			synth := chain.NewSynthesisRunner(oaiKey, "")

			session := repl.NewSession()
			cfg := repl.DispatchConfig{
				StageRunners:    runners,
				SynthesisRunner: synth,
				RepoPath:        repo,
			}
			return repl.Run(session, cfg)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Repo path (default: cwd)")
	return cmd
}
