class PRDescription {
  title string @description("Short PR title, ≤72 characters")
  summary string @description("2-4 sentence overview of what changed and why")
  changes string[] @description("Bullet list of key changes")
  test_plan string[] @description("Bullet list of how to verify the changes")
}

function DraftPR(prompt: string) -> PRDescription {
  client AnthropicBalanced
  prompt #"
    {{ prompt }}
  "#
}
