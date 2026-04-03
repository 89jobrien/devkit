// Package providers defines abstractions for LLM backends and task routing.
package providers

// Anthropic model IDs by tier.
const (
	ModelAnthropicFast         = "claude-haiku-4-5"
	ModelAnthropicBalanced     = "claude-sonnet-4-6"
	ModelAnthropicLargeContext = "claude-sonnet-4-5" // 1M context window
	ModelAnthropicCoding       = "claude-sonnet-4-6"
)

// OpenAI model IDs by tier (gpt-5.4 series).
const (
	ModelOpenAIFast     = "gpt-5.4-mini"
	ModelOpenAIBalanced = "gpt-5.4"
	ModelOpenAICoding   = "gpt-5.4"
)

// Gemini model IDs by tier (gemini-3 series).
const (
	ModelGeminiFast         = "gemini-3-flash-preview"
	ModelGeminiBalanced     = "gemini-3-pro-preview"
	ModelGeminiLargeContext = "gemini-3-pro-preview" // 1M+ context
)
