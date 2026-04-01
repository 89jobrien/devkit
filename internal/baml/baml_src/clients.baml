client<llm> AnthropicBalanced {
  provider anthropic
  options {
    model "claude-sonnet-4-6"
    api_key env.ANTHROPIC_API_KEY
  }
}

client<llm> OpenAIBalanced {
  provider openai
  options {
    model "gpt-5.4"
    api_key env.OPENAI_API_KEY
  }
}

client<llm> GeminiFast {
  provider google-ai
  options {
    model "gemini-3-flash-preview"
    api_key env.GEMINI_API_KEY
  }
}

retry_policy CouncilRetry {
  max_retries 2
  strategy {
    type exponential_backoff
    delay_ms 1000
    multiplier 2
  }
}
