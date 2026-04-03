// cmd/meta/main.go — standalone meta agent binary.
// Equivalent to `devkit meta` but callable directly from scripts.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	devlog "github.com/89jobrien/devkit/internal/infra/log"
	"github.com/89jobrien/devkit/internal/ai/meta"
	"github.com/89jobrien/devkit/internal/ai/providers"
	"github.com/89jobrien/devkit/internal/infra/tools"
)

func main() {
	task := strings.Join(os.Args[1:], " ")
	if task == "" {
		// Try stdin
		info, err := os.Stdin.Stat()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: could not stat stdin:", err)
			os.Exit(1)
		}
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

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: could not get working directory:", err)
		os.Exit(1)
	}
	agentTools := []tools.Tool{
		tools.ReadTool(wd),
		tools.GlobTool(wd),
		tools.GrepTool(wd),
		tools.BashTool(30_000, nil),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	runner := meta.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
		return router.AgentRunnerFor(providers.TierCoding, agentTools).Run(ctx, prompt, ts)
	})

	res, err := meta.Exec(ctx, task, devlog.GatherRepoContext(), "", runner, os.Stdout, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if res.LogPath != "" {
		fmt.Printf("\nLogged to: %s\n", res.LogPath)
	}
}
