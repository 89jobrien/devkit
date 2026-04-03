enum Confidence {
  High
  Medium
  Low
}

class FailingJob {
  name        string   @description("Name of the failing CI job or step")
  log         string   @description("Log output of the failing job")
}

class ReproductionSteps {
  steps        string[]   @description("Steps to reproduce the failure locally")
  context      string     @description("Context for the reproduction steps")
  environment  string     @description("Environment variables and configuration for the reproduction steps")
  tools        string[]   @description("Tools and dependencies required for the reproduction steps")
}

class RootCause {
  category    string   @description("Category of the root cause (e.g. 'dependency', 'configuration', 'code')")
  description  string   @description("Specific root cause of the failure")
}

class SuggestedFix {
  kind         string   @description("Kind of fix (e.g. 'revert', 'patch', 'update')")
  description  string   @description("Concrete fix suggestion")
}


class CITriageReport {
  failing_job         FailingJob   @description("Failing CI job or step")
  root_cause          RootCause   @description("Specific root cause of the failure")
  suggested_fix       SuggestedFix   @description("Concrete fix suggestion")
  reproduction_steps  ReproductionSteps   @description("Steps to reproduce the failure locally")
  confidence          Confidence   @description("High | Medium | Low")
}

function TriageCIFailure(log: string, repo_context: string) -> CITriageReport {
  client AnthropicBalanced
  prompt #"
    You are a CI/CD engineer. Analyze the following CI failure log and identify the root cause.
    Be specific — point to exact errors, file names, and line numbers where visible.

    {% if repo_context %}Repository context:
    {{ repo_context }}

    {% endif %}CI failure log:
    ```
    {{ log }}
    ```

    {{ ctx.output_format }}
  "#
}
