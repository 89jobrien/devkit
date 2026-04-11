// cmd/devkit/cmd_chain.go
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/89jobrien/devkit/internal/chain"
	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/spf13/cobra"
)

// newChainCmd constructs the `devkit chain` command.
// stageRunners and synthesisRunner are injected for testing; pass nil to use production wiring.
func newChainCmd(stageRunners map[string]chain.StageRunner, synthesisRunner chain.StageRunner) *cobra.Command {
	var repo, runID string
	cmd := &cobra.Command{
		Use:   "chain <stage>...",
		Short: "Run selected stages in canonical order, ending with gpt-5.4 synthesis",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate stage names first (fast path before building runners).
			if _, err := chain.SelectStages(args); err != nil {
				return err
			}

			antKey := os.Getenv("ANTHROPIC_API_KEY")
			oaiKey := os.Getenv("OPENAI_API_KEY")
			gemKey := os.Getenv("GEMINI_API_KEY")

			// Preflight.
			preErrs := chain.Preflight(chain.PreflightConfig{
				Stages:       args,
				AnthropicKey: antKey,
				OpenAIKey:    oaiKey,
				RepoPath:     repo,
			})
			if len(preErrs) > 0 {
				msgs := make([]string, len(preErrs))
				for i, e := range preErrs {
					msgs[i] = e.Error()
				}
				return fmt.Errorf("preflight failed:\n  %s", strings.Join(msgs, "\n  "))
			}

			// Build runners if not injected (production path).
			runners := stageRunners
			synth := synthesisRunner
			if runners == nil {
				runners = chain.BuildStageRunners(chain.StageWiringConfig{
					RepoPath:     repo,
					RunID:        runID,
					AnthropicKey: antKey,
					OpenAIKey:    oaiKey,
					GeminiKey:    gemKey,
				})
			}
			if synth == nil {
				synth = chain.NewSynthesisRunner(oaiKey, "")
			}

			// Build slot list with runners wired.
			slots, err := chain.SelectStages(args)
			if err != nil {
				return err
			}
			for i, slot := range slots {
				if slot.Selected {
					if r, ok := runners[slot.Name]; ok {
						slots[i].Runner = r
					}
				}
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("chain", map[string]string{"stages": strings.Join(args, ",")})
			start := time.Now()

			results, err := chain.RunPipeline(cmd.Context(), slots, synth)
			if err != nil {
				return err
			}

			// Print each non-skipped result.
			for _, r := range results {
				if r.IsSkipped() {
					continue
				}
				if r.Err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "\n## %s (FAILED: %v)\n\n", r.Stage, r.Err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n## %s\n\n%s\n", r.Stage, r.Output)
			}

			last := results[len(results)-1]
			devlog.Complete(id, "chain", map[string]string{"stages": strings.Join(args, ",")}, last.Output, time.Since(start))
			_, _ = devlog.SaveCommitLog(sha, "chain", last.Output, map[string]string{"stages": strings.Join(args, ",")})
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Repo path (default: cwd)")
	cmd.Flags().StringVar(&runID, "run", "", "GitHub Actions run ID for ci-triage stage")
	return cmd
}
