// internal/platform/platform.go
package platform

import (
	"context"
	"fmt"
)

// JobLog holds the name and log text of a failed CI job.
type JobLog struct {
	Name string
	Log  string
}

const maxLogBytes = 30_000

// Platform is the port for CI platform operations.
type Platform interface {
	SetCommitStatus(ctx context.Context, state, description string) error
	EnsureLabelExists(ctx context.Context) error
	FindIssueForCommit(ctx context.Context, sha string) (int, bool, error)
	CreateIssue(ctx context.Context, sha, diagnosis, provider string, failedJobs []string, runID string) (int, error)
	AddComment(ctx context.Context, issueNumber int, diagnosis, provider string) error
	FetchFailedJobLogs(ctx context.Context, runID string) ([]JobLog, error)
}

// New returns the appropriate Platform adapter based on name ("gitea" or "github").
// baseURL is required for gitea; for github it defaults to https://api.github.com.
// Passing a non-empty baseURL for github overrides the default (useful in tests).
func New(name, repo, runID, commitSHA, token, baseURL string) (Platform, error) {
	switch name {
	case "gitea":
		if baseURL == "" {
			return nil, fmt.Errorf("gitea requires GITEA_URL")
		}
		return &giteaPlatform{repo: repo, runID: runID, sha: commitSHA, token: token, baseURL: baseURL}, nil
	case "github":
		base := baseURL
		if base == "" {
			base = "https://api.github.com"
		}
		return &githubPlatform{repo: repo, runID: runID, sha: commitSHA, token: token, baseURL: base}, nil
	default:
		return nil, fmt.Errorf("unknown platform: %q (want gitea or github)", name)
	}
}

func truncateLast(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "...[truncated]...\n" + s[len(s)-n:]
}
