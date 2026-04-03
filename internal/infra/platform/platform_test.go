// internal/platform/platform_test.go
package platform_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/89jobrien/devkit/internal/infra/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGiteaFetchFailedJobLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/owner/repo/actions/runs/42/jobs":
			json.NewEncoder(w).Encode(map[string]any{
				"jobs": []map[string]any{
					{"id": 1, "name": "lint", "conclusion": "failure"},
					{"id": 2, "name": "test", "conclusion": "success"},
				},
			})
		case "/api/v1/repos/owner/repo/actions/jobs/1/logs":
			w.Write([]byte("error: undefined variable"))
		}
	}))
	defer srv.Close()

	p, err := platform.New("gitea", "owner/repo", "42", "abc123", "tok", srv.URL)
	require.NoError(t, err)

	logs, err := p.FetchFailedJobLogs(context.Background(), "42")
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "lint", logs[0].Name)
	assert.Contains(t, logs[0].Log, "error: undefined variable")
}

func TestGiteaCreateIssue(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/owner/repo/labels":
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode([]map[string]any{})
			} else {
				json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "ci-failure"})
			}
		case "/api/v1/repos/owner/repo/issues":
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode([]map[string]any{})
			} else {
				json.NewDecoder(r.Body).Decode(&capturedBody)
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]any{"number": 7})
			}
		case "/api/v1/repos/owner/repo/statuses/abc123":
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer srv.Close()

	p, err := platform.New("gitea", "owner/repo", "42", "abc123", "tok", srv.URL)
	require.NoError(t, err)

	require.NoError(t, p.EnsureLabelExists(context.Background()))
	num, err := p.CreateIssue(context.Background(), "abc123", "root cause: X", "anthropic", []string{"lint"}, "42")
	require.NoError(t, err)
	assert.Equal(t, 7, num)
	assert.Contains(t, capturedBody["body"], "<!-- sha: abc123 -->")
}

func TestGitHubFetchFailedJobLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/actions/runs/42/jobs":
			json.NewEncoder(w).Encode(map[string]any{
				"jobs": []map[string]any{
					{"id": 10, "name": "build", "conclusion": "failure"},
				},
			})
		case "/repos/owner/repo/actions/jobs/10/logs":
			w.Write([]byte("FAIL: compilation error"))
		}
	}))
	defer srv.Close()

	p, err := platform.New("github", "owner/repo", "42", "abc123", "ghp_token", srv.URL)
	require.NoError(t, err)

	logs, err := p.FetchFailedJobLogs(context.Background(), "42")
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "build", logs[0].Name)
}
