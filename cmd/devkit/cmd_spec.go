// cmd/devkit/cmd_spec.go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/89jobrien/devkit/internal/ai/providers"
	"github.com/89jobrien/devkit/internal/ai/spec"
	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/spf13/cobra"
)

const defaultSpecModel = "gpt-5.4-mini"
const defaultSynthesisModel = "gpt-5.4"
const defaultSpecsDir = "docs/superpowers/specs"

// newSpecCmd returns the spec subcommand. roleRunner and synthRunner are
// injected for testing; pass nil to have the command build them from config.
func newSpecCmd(roleRunner spec.Runner, synthRunner spec.Runner) *cobra.Command {
	var noSynth bool
	cmd := &cobra.Command{
		Use:   "spec [path]",
		Short: "Multi-role spec review",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()

			rr, sr := roleRunner, synthRunner
			if rr == nil || sr == nil {
				cfg, err := LoadConfig()
				if err != nil {
					return err
				}
				if cfg.Project.Name != "" {
					os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
				}
				roleModel := cfg.Spec.RoleModel
				if roleModel == "" {
					roleModel = defaultSpecModel
				}
				synthModel := cfg.Spec.SynthesisModel
				if synthModel == "" {
					synthModel = defaultSynthesisModel
				}
				apiKey := os.Getenv("OPENAI_API_KEY")
				rr = newOpenAISpecRunner(apiKey, roleModel)
				sr = newOpenAISpecRunner(apiKey, synthModel)
			}

			var specPath string
			if len(args) == 1 {
				specPath = args[0]
			} else {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				dir := filepath.Join(wd, defaultSpecsDir)
				specPath, err = spec.LatestSpecFile(dir)
				if err != nil {
					return fmt.Errorf("spec: auto-discover: %w", err)
				}
				fmt.Fprintf(w, "Using latest spec: %s\n\n", specPath)
			}

			content, err := os.ReadFile(specPath)
			if err != nil {
				return fmt.Errorf("spec: read file: %w", err)
			}

			sha := devlog.GitShortSHA()
			id := devlog.Start("spec", map[string]string{"path": specPath})
			start := time.Now()

			result, err := spec.Run(cmd.Context(), spec.Config{
				Content: string(content),
				Path:    specPath,
				Runner:  rr,
			})
			if err != nil {
				return err
			}

			roleOrder := []string{"completeness", "ambiguity", "scope", "critic", "creative", "generalist"}
			var allOutput strings.Builder
			for _, key := range roleOrder {
				out, ok := result.RoleOutputs[key]
				if !ok {
					continue
				}
				fmt.Fprintf(w, "\n---- %s ----\n%s\n", key, out)
				allOutput.WriteString(fmt.Sprintf("## %s\n%s\n\n", key, out))
			}

			if !noSynth {
				synthesis, err := spec.Synthesize(cmd.Context(), result.RoleOutputs, specPath, sr)
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "\n---- SYNTHESIS ----\n%s\n", synthesis)
				allOutput.WriteString(fmt.Sprintf("## Synthesis\n%s\n", synthesis))
			}

			score := spec.MetaScore(result.RoleOutputs)
			fmt.Fprintf(w, "\nMeta Health Score: %.0f%%\n", score*100)

			devlog.Complete(id, "spec", map[string]string{"path": specPath}, allOutput.String(), time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "spec", allOutput.String(), map[string]string{"path": specPath})
			fmt.Fprintf(w, "\nLogged to: %s\n", path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&noSynth, "no-synthesis", false, "Skip synthesis")
	return cmd
}

// newOpenAISpecRunner constructs a spec.Runner backed by the OpenAI provider.
func newOpenAISpecRunner(apiKey, model string) spec.Runner {
	p := providers.NewOpenAIProvider(apiKey, model, "")
	return spec.RunnerFunc(func(ctx context.Context, prompt string, _ []string) (string, error) {
		return p.Chat(ctx, prompt)
	})
}
