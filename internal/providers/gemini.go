// Package providers defines abstractions for LLM backends and task routing.
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GeminiProvider implements ChatProvider using the Gemini generateContent API.
// Gemini is used for large-context ingestion; tool use is not supported here.
type GeminiProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewGeminiProvider constructs a GeminiProvider.
// baseURL defaults to "https://generativelanguage.googleapis.com" if empty.
func NewGeminiProvider(apiKey, model, baseURL string) *GeminiProvider {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	return &GeminiProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *GeminiProvider) Chat(ctx context.Context, prompt string) (string, error) {
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", p.baseURL, p.model)
	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{{
			"parts": []map[string]any{{"text": prompt}},
		}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("gemini: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gemini HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("gemini: decode response: %w", err)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: no candidates in response")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}
