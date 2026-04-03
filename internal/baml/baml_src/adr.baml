class ADROutput {
  status      string   @description("Proposed | Accepted | Deprecated | Superseded")
  context     string   @description("Background, forces, and problem being solved")
  decision    string   @description("The chosen approach and rationale")
  consequences string[] @description("Trade-offs, risks, and follow-on effects")
}

function DraftADR(title: string, ctx_text: string) -> ADROutput {
  client AnthropicBalanced
  prompt #"
    You are a senior software architect. Draft a concise Architecture Decision Record.

    Title: {{ title }}

    {% if ctx_text %}Context/Problem:
    {{ ctx_text }}
    {% endif %}

    Fill every field. Be specific and actionable. Keep each field under 150 words.

    {{ ctx.output_format }}
  "#
}
