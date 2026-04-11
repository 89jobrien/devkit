// cmd/devkit/cmd_repl.go
package main

import (
	"fmt"
	"os"

	"github.com/89jobrien/devkit/internal/chain"
	"github.com/89jobrien/devkit/internal/repl"
	"github.com/spf13/cobra"
)

// resolveRepo returns repoPath if non-empty, otherwise the current working directory.
func resolveRepo(repoPath string) string {
	if repoPath != "" {
		return repoPath
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func newReplCmd() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:   "repl",
		Short: "Start an interactive REPL session with persistent auth and context",
		RunE: func(cmd *cobra.Command, args []string) error {
			antKey := os.Getenv("ANTHROPIC_API_KEY")
			oaiKey := os.Getenv("OPENAI_API_KEY")
			gemKey := os.Getenv("GEMINI_API_KEY")

			// OPENAI_API_KEY is required: synthesis always uses gpt-5.4 via OpenAI.
			// ANTHROPIC_API_KEY is optional (used for non-synthesis stages).
			if oaiKey == "" {
				return fmt.Errorf("repl: OPENAI_API_KEY required (synthesis stage always uses gpt-5.4)")
			}

			repoPath := resolveRepo(repo)
			runners := chain.BuildStageRunners(chain.StageWiringConfig{
				RepoPath:     repoPath,
				AnthropicKey: antKey,
				OpenAIKey:    oaiKey,
				GeminiKey:    gemKey,
			})
			synth := chain.NewSynthesisRunner(oaiKey, "")

			session := repl.NewSession()
			cfg := repl.DispatchConfig{
				StageRunners:    runners,
				SynthesisRunner: synth,
				RepoPath:        repoPath,
			}
			return repl.Run(session, cfg)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Repo path (default: cwd)")
	return cmd
}
