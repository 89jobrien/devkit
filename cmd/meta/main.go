// cmd/meta/main.go — standalone meta agent binary.
// Equivalent to `devkit meta` but callable directly from scripts.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	devlog "github.com/89jobrien/devkit/internal/log"
	"github.com/89jobrien/devkit/internal/meta"
	"github.com/89jobrien/devkit/internal/providers"
	"github.com/89jobrien/devkit/internal/tools"
)

func main() {
	task := strings.Join(os.Args[1:], " ")
	if task == "" {
		// Try stdin
		info, _ := os.Stdin.Stat()
		if info.Mode()&os.ModeCharDevice == 0 {
			var sb strings.Builder
			buf := make([]byte, 4096)
			for {
				n, err := os.Stdin.Read(buf)
				sb.Write(buf[:n])
				if err != nil {
					break
				}
			}
			task = strings.TrimSpace(sb.String())
		}
	}
	if task == "" {
		fmt.Fprintln(os.Stderr, "usage: meta <task> or echo <task> | meta")
		os.Exit(1)
	}

	router := providers.NewRouter(providers.RouterConfig{
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIKey:    os.Getenv("OPENAI_API_KEY"),
		GeminiKey:    os.Getenv("GEMINI_API_KEY"),
	})

	wd, _ := os.Getwd()
	agentTools := []tools.Tool{
		tools.ReadTool(wd),
		tools.GlobTool(wd),
		tools.GrepTool(wd),
		tools.BashTool(30_000, nil),
	}

	sha := devlog.GitShortSHA()
	taskPreview := task
	if len(taskPreview) > 80 {
		taskPreview = taskPreview[:80]
	}
	id := devlog.Start("meta", map[string]string{"task": taskPreview})
	start := time.Now()

	result, err := meta.Run(context.Background(), task, devlog.GatherRepoContext(), "",
		meta.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
			return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
		}))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	var allOutput strings.Builder
	for name, out := range result.Outputs {
		fmt.Printf("\n---- %s ----\n%s\n", name, out)
		allOutput.WriteString(fmt.Sprintf("## %s\n%s\n\n", name, out))
	}
	fmt.Printf("\n---- SYNTHESIS ----\n%s\n", result.Summary)
	allOutput.WriteString(fmt.Sprintf("## Synthesis\n%s\n", result.Summary))

	devlog.Complete(id, "meta", map[string]string{"task": taskPreview}, allOutput.String(), time.Since(start))
	path, _ := devlog.SaveCommitLog(sha, "meta", allOutput.String(), map[string]string{"task": taskPreview})
	fmt.Printf("\nLogged to: %s\n", path)
}
