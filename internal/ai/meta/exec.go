package meta

import (
	"context"
	"fmt"
	"io"
	"time"

	devlog "github.com/89jobrien/devkit/internal/infra/log"
)

// ExecResult holds the log path after a successful run.
type ExecResult struct {
	LogPath string
}

// Exec runs the meta-agent flow and handles output and log persistence.
func Exec(
	ctx context.Context,
	task string,
	repoContext string,
	sdkDocs string,
	runner Runner,
	w io.Writer,
	noSynth bool,
) (*ExecResult, error) {
	taskPreview := task
	if len(taskPreview) > 80 {
		taskPreview = taskPreview[:80]
	}

	logMeta := map[string]string{"task": taskPreview}
	sha := devlog.GitShortSHA()
	id := devlog.Start("meta", logMeta)
	start := time.Now()

	result, err := Run(ctx, task, repoContext, sdkDocs, runner)
	if err != nil {
		return nil, err
	}

	combined := ""
	for name, output := range result.Outputs {
		fmt.Fprintf(w, "\n---- %s ----\n%s\n", name, output)
		combined += fmt.Sprintf("## %s\n%s\n\n", name, output)
	}
	if !noSynth {
		fmt.Fprintf(w, "\n---- SYNTHESIS ----\n%s\n", result.Summary)
		combined += fmt.Sprintf("## Synthesis\n%s\n", result.Summary)
	}

	devlog.Complete(id, "meta", logMeta, combined, time.Since(start))
	logPath, logErr := devlog.SaveCommitLog(sha, "meta", combined, logMeta)
	if logErr != nil {
		fmt.Fprintf(w, "\nwarning: could not save log: %v\n", logErr)
	}

	return &ExecResult{LogPath: logPath}, nil
}
