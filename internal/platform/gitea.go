// internal/platform/gitea.go
package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type giteaPlatform struct {
	repo, runID, sha, token, baseURL string
}

func (g *giteaPlatform) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.baseURL+"/api/v1"+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+g.token)
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func (g *giteaPlatform) FetchFailedJobLogs(ctx context.Context, runID string) ([]JobLog, error) {
	resp, err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/actions/runs/%s/jobs?limit=50", g.repo, runID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Jobs []struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			Conclusion string `json:"conclusion"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var logs []JobLog
	for _, j := range data.Jobs {
		if j.Conclusion != "failure" {
			continue
		}
		lr, err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/actions/jobs/%d/logs", g.repo, j.ID), nil)
		if err != nil {
			logs = append(logs, JobLog{Name: j.Name, Log: fmt.Sprintf("(log unavailable: %v)", err)})
			continue
		}
		raw, _ := io.ReadAll(lr.Body)
		lr.Body.Close()
		logs = append(logs, JobLog{Name: j.Name, Log: truncateLast(string(raw), maxLogBytes)})
	}
	return logs, nil
}

func (g *giteaPlatform) SetCommitStatus(ctx context.Context, state, description string) error {
	if len(description) > 140 {
		description = description[:140]
	}
	resp, err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/statuses/%s", g.repo, g.sha), map[string]string{
		"context":     "ci/agent-diagnosis",
		"state":       state,
		"description": description,
	})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (g *giteaPlatform) EnsureLabelExists(ctx context.Context) error {
	resp, err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/labels", g.repo), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var labels []struct{ Name string `json:"name"` }
	json.NewDecoder(resp.Body).Decode(&labels)
	for _, l := range labels {
		if l.Name == "ci-failure" {
			return nil
		}
	}
	cr, err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/labels", g.repo), map[string]string{
		"name": "ci-failure", "color": "#e11d48",
	})
	if err != nil {
		return err
	}
	cr.Body.Close()
	return nil
}

func (g *giteaPlatform) FindIssueForCommit(ctx context.Context, sha string) (int, bool, error) {
	marker := fmt.Sprintf("<!-- sha: %s -->", sha)
	for page := 1; ; page++ {
		resp, err := g.do(ctx, http.MethodGet,
			fmt.Sprintf("/repos/%s/issues?state=open&type=issues&labels=ci-failure&limit=50&page=%d", g.repo, page), nil)
		if err != nil {
			return 0, false, err
		}
		var issues []struct {
			Number int    `json:"number"`
			Body   string `json:"body"`
		}
		json.NewDecoder(resp.Body).Decode(&issues)
		resp.Body.Close()
		if len(issues) == 0 {
			return 0, false, nil
		}
		for _, i := range issues {
			if strings.Contains(i.Body, marker) {
				return i.Number, true, nil
			}
		}
	}
}

func (g *giteaPlatform) CreateIssue(ctx context.Context, sha, diagnosis, provider string, failedJobs []string, runID string) (int, error) {
	shortSHA := sha
	if len(shortSHA) > 8 {
		shortSHA = shortSHA[:8]
	}
	title := fmt.Sprintf("CI failure: %s — %s", shortSHA, strings.Join(failedJobs, ", "))
	runURL := fmt.Sprintf("%s/%s/actions/runs/%s", g.baseURL, g.repo, runID)
	body := fmt.Sprintf("## CI Failure Diagnosis\n\n**Jobs:** %s\n**Provider:** %s\n**Commit:** %s\n**Run:** [%s](%s)\n\n%s\n\n---\n*Diagnosed by ci-agent.*\n<!-- sha: %s -->",
		strings.Join(failedJobs, ", "), provider, sha, runID, runURL, diagnosis, sha)

	resp, err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues", g.repo), map[string]any{
		"title": title, "body": body, "labels": []string{"ci-failure"},
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var result struct{ Number int `json:"number"` }
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Number, nil
}

func (g *giteaPlatform) AddComment(ctx context.Context, issueNumber int, diagnosis, provider string) error {
	body := fmt.Sprintf("## Re-run Diagnosis\n\n**Provider:** %s\n\n%s", provider, diagnosis)
	resp, err := g.do(ctx, http.MethodPost,
		fmt.Sprintf("/repos/%s/issues/%d/comments", g.repo, issueNumber),
		map[string]string{"body": body})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
