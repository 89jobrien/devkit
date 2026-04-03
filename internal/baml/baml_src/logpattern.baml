class LogPatternEntry {
  pattern     string @description("The recurring error pattern or signature")
  count       int    @description("Approximate occurrence count")
  severity    string @description("critical|high|medium|low|info")
  first_seen  string @description("Timestamp or context of first occurrence")
  last_seen   string @description("Timestamp or context of last occurrence")
  suggestion  string @description("Actionable suggestion to address this pattern")
}

class LogPatternOutput {
  summary  string          @description("2-3 sentence overview of log health")
  patterns LogPatternEntry[] @description("Recurring patterns sorted by severity then frequency")
}

function AnalyzeLogPatterns(logs: string) -> LogPatternOutput {
  client AnthropicBalanced
  prompt #"
    You are an SRE analyzing logs for recurring error patterns.

    Logs:
    ```
    {{ logs }}
    ```

    Identify recurring error patterns, group by type, and provide actionable suggestions.

    {{ ctx.output_format }}
  "#
}
