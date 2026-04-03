class HealthCheck {
  name        string  @description("Name of the check")
  status      string  @description("pass | warn | fail")
  severity    string  @description("info | warning | critical")
  detail      string  @description("What was found")
  suggestion  string  @description("What to do about it")
}

class HealthReport {
  repo        string        @description("Repository path or name")
  score       int           @description("Overall health score 0-100")
  summary     string        @description("One-paragraph plain-language summary")
  checks      HealthCheck[] @description("Individual check results")
}

function AnalyzeRepoHealth(repo_context: string, check_results: string) -> HealthReport {
  client AnthropicBalanced
  prompt #"
    You are a senior engineer reviewing repository health. Given the check results below, produce
    a scored health report. Score from 0 (critical issues) to 100 (excellent). Be concise.

    Repository context:
    {{ repo_context }}

    Check results:
    {{ check_results }}

    {{ ctx.output_format }}
  "#
}
