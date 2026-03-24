// cmd/devkit/main.go
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/89jobrien/devkit/internal/council"
	devlog "github.com/89jobrien/devkit/internal/log"
	"github.com/89jobrien/devkit/internal/meta"
	"github.com/89jobrien/devkit/internal/review"
	"github.com/spf13/cobra"
)

func gitDiff(base string) string {
	out, err := exec.Command("git", "diff", base+"...HEAD").Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		out, _ = exec.Command("git", "diff", "HEAD").Output()
	}
	return string(out)
}

func gitLog(base string) string {
	out, _ := exec.Command("git", "log", base+"...HEAD", "--oneline").Output()
	return string(out)
}

func gatherRepoContext() string {
	run := func(args ...string) string {
		out, _ := exec.Command(args[0], args[1:]...).Output()
		return string(out)
	}
	var sb strings.Builder
	for _, f := range []string{"CLAUDE.md", "AGENTS.md", "README.md"} {
		if data, err := os.ReadFile(f); err == nil {
			preview := data
			if len(preview) > 2000 {
				preview = preview[:2000]
			}
			sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", f, string(preview)))
		}
	}
	sb.WriteString("## Recent commits\n" + run("git", "log", "--oneline", "-20"))
	sb.WriteString("\n## Working tree\n" + run("git", "status", "--short"))
	allFiles := run("git", "ls-files")
	lines := strings.Split(strings.TrimSpace(allFiles), "\n")
	if len(lines) > 150 {
		lines = lines[:150]
	}
	sb.WriteString("\n## Structure (first 150 paths)\n" + strings.Join(lines, "\n"))
	return sb.String()
}

var sdkDocURLs = []string{
	"https://docs.anthropic.com/en/docs/claude-code/sdk",
	"https://docs.anthropic.com/en/docs/claude-code/sdk/sdk-python",
}

func fetchSDKDocs(forceRefresh bool) string {
	home, _ := os.UserHomeDir()
	cacheDir := home + "/.dev-agents/cache"
	cachePath := cacheDir + "/sdk-docs.md"

	if !forceRefresh {
		if info, err := os.Stat(cachePath); err == nil {
			if time.Since(info.ModTime()) < 24*time.Hour {
				data, _ := os.ReadFile(cachePath)
				return string(data)
			}
		}
	}

	_ = os.MkdirAll(cacheDir, 0o755)

	var sections []string
	client := &http.Client{Timeout: 15 * time.Second}
	for _, url := range sdkDocURLs {
		resp, err := client.Get(url)
		if err != nil {
			sections = append(sections, fmt.Sprintf("<!-- failed to fetch %s: %v -->", url, err))
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		text := htmlToText(string(raw))
		if len(text) > 20000 {
			text = text[:20000]
		}
		sections = append(sections, fmt.Sprintf("<!-- %s -->\n%s", url, text))
	}

	content := strings.Join(sections, "\n\n---\n\n")
	_ = os.WriteFile(cachePath, []byte(content), 0o644)
	return content
}

func htmlToText(html string) string {
	re := regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	html = re.ReplaceAllString(html, "")
	tags := regexp.MustCompile(`<[^>]+>`)
	text := tags.ReplaceAllString(html, " ")
	ws := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(ws.ReplaceAllString(text, " "))
}

func main() {
	root := &cobra.Command{
		Use:   "devkit",
		Short: "AI-powered dev workflow toolkit",
	}

	// council subcommand
	var councilBase, councilMode string
	var councilNoSynth bool
	councilCmd := &cobra.Command{
		Use:   "council",
		Short: "Multi-role branch analysis",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			diff := gitDiff(councilBase)
			commits := gitLog(councilBase)
			runner := newAgentRunner()
			sha := devlog.GitShortSHA()

			id := devlog.Start("council", map[string]string{"base": councilBase, "mode": councilMode})
			start := time.Now()

			result, err := council.Run(cmd.Context(), council.Config{
				Base:    councilBase,
				Mode:    councilMode,
				Diff:    diff,
				Commits: commits,
				Runner: council.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return runner.Run(ctx, prompt, ts)
				}),
			})
			if err != nil {
				return err
			}

			var allOutput strings.Builder
			for key, out := range result.RoleOutputs {
				fmt.Printf("\n---- %s ----\n%s\n", key, out)
				allOutput.WriteString(fmt.Sprintf("## %s\n%s\n\n", key, out))
			}

			if !councilNoSynth {
				synthesis, err := council.Synthesize(cmd.Context(), result.RoleOutputs, council.Config{
					Base: councilBase, Diff: diff, Commits: commits,
				}, council.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return runner.Run(ctx, prompt, ts)
				}))
				if err != nil {
					return err
				}
				fmt.Printf("\n---- SYNTHESIS ----\n%s\n", synthesis)
				allOutput.WriteString(fmt.Sprintf("## Synthesis\n%s\n", synthesis))
			}

			score := council.MetaScore(result.RoleOutputs)
			fmt.Printf("\nMeta Health Score: %.0f%%\n", score*100)

			devlog.Complete(id, "council", map[string]string{"base": councilBase, "mode": councilMode},
				allOutput.String(), time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "council", allOutput.String(), map[string]string{
				"base": councilBase, "mode": councilMode,
			})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	councilCmd.Flags().StringVar(&councilBase, "base", "main", "Base branch/ref")
	councilCmd.Flags().StringVar(&councilMode, "mode", "core", "core or extensive")
	councilCmd.Flags().BoolVar(&councilNoSynth, "no-synthesis", false, "Skip synthesis")

	// review subcommand
	var reviewBase string
	reviewCmd := &cobra.Command{
		Use:   "review",
		Short: "AI diff review",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			diff := gitDiff(reviewBase)
			if strings.TrimSpace(diff) == "" {
				fmt.Printf("No changes versus %s — nothing to review.\n", reviewBase)
				return nil
			}

			runner := newAgentRunner()
			sha := devlog.GitShortSHA()
			id := devlog.Start("review", map[string]string{"base": reviewBase})
			start := time.Now()

			result, err := review.Run(cmd.Context(), review.Config{
				Base:  reviewBase,
				Diff:  diff,
				Focus: cfg.Review.Focus,
				Runner: review.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return runner.Run(ctx, prompt, ts)
				}),
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			devlog.Complete(id, "review", map[string]string{"base": reviewBase}, result, time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "review", result, map[string]string{"base": reviewBase})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	reviewCmd.Flags().StringVar(&reviewBase, "base", "main", "Base branch/ref")

	// meta subcommand
	var metaNoSynth, metaRefreshDocs bool
	metaCmd := &cobra.Command{
		Use:   "meta [task]",
		Short: "Design and run parallel agents for any task",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			task := ""
			if len(args) > 0 {
				task = args[0]
			} else {
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
				return fmt.Errorf("provide a task as argument or via stdin")
			}

			cfg, _ := LoadConfig()
			if cfg.Project.Name != "" {
				os.Setenv("DEVKIT_PROJECT", cfg.Project.Name)
			}

			repoContext := gatherRepoContext()
			sdkDocs := fetchSDKDocs(metaRefreshDocs)
			runner := newAgentRunner()
			sha := devlog.GitShortSHA()

			taskPreview := task
			if len(taskPreview) > 80 {
				taskPreview = taskPreview[:80]
			}

			id := devlog.Start("meta", map[string]string{"task": taskPreview})
			start := time.Now()

			result, err := meta.Run(cmd.Context(), task, repoContext, sdkDocs,
				meta.RunnerFunc(func(ctx context.Context, prompt string, ts []string) (string, error) {
					return runner.Run(ctx, prompt, ts)
				}))
			if err != nil {
				return err
			}

			var allOutput strings.Builder
			for name, out := range result.Outputs {
				fmt.Printf("\n---- %s ----\n%s\n", name, out)
				allOutput.WriteString(fmt.Sprintf("## %s\n%s\n\n", name, out))
			}
			if !metaNoSynth {
				fmt.Printf("\n---- SYNTHESIS ----\n%s\n", result.Summary)
				allOutput.WriteString(fmt.Sprintf("## Synthesis\n%s\n", result.Summary))
			}

			devlog.Complete(id, "meta", map[string]string{"task": taskPreview},
				allOutput.String(), time.Since(start))
			path, _ := devlog.SaveCommitLog(sha, "meta", allOutput.String(), map[string]string{"task": taskPreview})
			fmt.Printf("\nLogged to: %s\n", path)
			return nil
		},
	}
	metaCmd.Flags().BoolVar(&metaNoSynth, "no-synthesis", false, "Skip synthesis")
	metaCmd.Flags().BoolVar(&metaRefreshDocs, "refresh-docs", false, "Force re-fetch SDK docs")

	root.AddCommand(councilCmd, reviewCmd, metaCmd)
	if err := root.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}
