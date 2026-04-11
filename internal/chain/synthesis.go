// internal/chain/synthesis.go
package chain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const synthesisModel = "gpt-5.4"

// SynthesisRunner is the always-last pipeline stage powered by OpenAI gpt-5.4.
type SynthesisRunner struct {
	apiKey  string
	baseURL string // overridable for tests; empty means production
}

// NewSynthesisRunner constructs a SynthesisRunner. Pass a non-empty baseURL to
// redirect requests to an httptest server in tests.
func NewSynthesisRunner(apiKey, baseURL string) *SynthesisRunner {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &SynthesisRunner{apiKey: apiKey, baseURL: baseURL}
}

// Run synthesizes all prior stage results into a final coherent summary.
// It always runs, even if prior stages errored — partial context is better than none.
func (s *SynthesisRunner) Run(ctx context.Context, prior []Result) Result {
	prompt := buildSynthesisPrompt(prior)
	output, err := s.callOpenAI(ctx, prompt)
	if err != nil {
		return Result{Stage: "synthesis", Err: fmt.Errorf("synthesis: %w", err)}
	}
	return Result{
		Stage:   "synthesis",
		Output:  output,
		Payload: &SynthesisPayload{Summary: output},
	}
}

func buildSynthesisPrompt(prior []Result) string {
	var sb strings.Builder
	sb.WriteString("You are a senior engineering lead synthesizing the output of a multi-stage\n")
	sb.WriteString("automated analysis pipeline. Produce a concise, actionable summary covering:\n")
	sb.WriteString("key findings, root causes, and recommended next actions.\n\n")
	sb.WriteString("## Pipeline Results\n\n")
	for _, r := range prior {
		if r.IsSkipped() {
			sb.WriteString("- [skipped]\n")
			continue
		}
		if r.Err != nil {
			fmt.Fprintf(&sb, "### %s (FAILED: %v)\n\n", r.Stage, r.Err)
			continue
		}
		fmt.Fprintf(&sb, "### %s\n\n%s\n\n", r.Stage, r.Output)
	}
	sb.WriteString("---\nProvide your synthesis now.")
	return sb.String()
}

func (s *SynthesisRunner) callOpenAI(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": synthesisModel,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
		"max_completion_tokens": 2048,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return out.Choices[0].Message.Content, nil
}
