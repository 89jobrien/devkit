// cmd/ci-agent/providers.go
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

var errDiagnosisUnavailable = errors.New("all LLM providers failed or have no API keys")

const maxTokens = 1024

// askWithFallback tries Anthropic -> OpenAI -> Gemini.
// baseURLOverride optionally redirects the Anthropic endpoint (used in tests).
func askWithFallback(prompt string, baseURLOverride ...string) (text, provider string, err error) {
	anthropicBase := "https://api.anthropic.com"
	if len(baseURLOverride) > 0 && baseURLOverride[0] != "" {
		anthropicBase = baseURLOverride[0]
	}

	type providerFn func(string, string) (string, error)
	providers := []struct {
		name string
		key  string
		fn   providerFn
	}{
		{"anthropic/claude-sonnet-4-6", os.Getenv("ANTHROPIC_API_KEY"), func(p, key string) (string, error) {
			return askAnthropic(p, key, anthropicBase)
		}},
		{"openai/gpt-4.1", os.Getenv("OPENAI_API_KEY"), askOpenAI},
		{"google/gemini-2.5-flash", os.Getenv("GEMINI_API_KEY"), askGemini},
	}

	var errs []string
	for _, prov := range providers {
		if prov.key == "" {
			continue
		}
		t, e := prov.fn(prompt, prov.key)
		if e != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", prov.name, e))
			continue
		}
		return t, prov.name, nil
	}
	if len(errs) > 0 {
		return "", "", fmt.Errorf("%w: %s", errDiagnosisUnavailable, strings.Join(errs, "; "))
	}
	return "", "", errDiagnosisUnavailable
}

func postJSON(url string, headers map[string]string, body any) ([]byte, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func askAnthropic(prompt, key, baseURL string) (string, error) {
	raw, err := postJSON(baseURL+"/v1/messages",
		map[string]string{"x-api-key": key, "anthropic-version": "2023-06-01"},
		map[string]any{
			"model":      "claude-sonnet-4-6",
			"max_tokens": maxTokens,
			"messages":   []map[string]string{{"role": "user", "content": prompt}},
		})
	if err != nil {
		return "", err
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	for _, c := range resp.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", errors.New("no text in response")
}

func askOpenAI(prompt, key string) (string, error) {
	raw, err := postJSON("https://api.openai.com/v1/chat/completions",
		map[string]string{"Authorization": "Bearer " + key},
		map[string]any{
			"model":      "gpt-4.1",
			"max_tokens": maxTokens,
			"messages":   []map[string]string{{"role": "user", "content": prompt}},
		})
	if err != nil {
		return "", err
	}
	var resp struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("no choices in response")
	}
	return resp.Choices[0].Message.Content, nil
}

func askGemini(prompt, key string) (string, error) {
	raw, err := postJSON(
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent",
		map[string]string{"x-goog-api-key": key},
		map[string]any{"contents": []map[string]any{{"parts": []map[string]string{{"text": prompt}}}}},
	)
	if err != nil {
		return "", err
	}
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct{ Text string `json:"text"` } `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("no candidates in response")
	}
	return resp.Candidates[0].Content.Parts[0].Text, nil
}
