class FindingSeverity {
  description string
  severity string @description("critical|high|medium|low|info")
}

class RoleOutput {
  health_score float
  summary string
  recommendations string[]
}

class StrictCriticOutput {
  health_score float
  summary string
  recommendations string[]
  risks string[]
}

class CreativeExplorerOutput {
  health_score float
  summary string
  recommendations string[]
  innovation_opportunities string[]
  architectural_potential string
}

class SecurityReviewerOutput {
  health_score float
  summary string
  recommendations string[]
  findings FindingSeverity[]
}

class GeneralAnalystOutput {
  health_score float
  summary string
  recommendations string[]
  gaps string[]
  work_patterns string
}

function AnalyzeBranchStrictCritic(prompt: string) -> StrictCriticOutput {
  client AnthropicBalanced
  prompt #"
    {{ prompt }}
  "#
}

function AnalyzeBranchCreativeExplorer(prompt: string) -> CreativeExplorerOutput {
  client AnthropicBalanced
  prompt #"
    {{ prompt }}
  "#
}

function AnalyzeBranchSecurityReviewer(prompt: string) -> SecurityReviewerOutput {
  client AnthropicBalanced
  prompt #"
    {{ prompt }}
  "#
}

function AnalyzeBranchGeneralAnalyst(prompt: string) -> GeneralAnalystOutput {
  client AnthropicBalanced
  prompt #"
    {{ prompt }}
  "#
}

function AnalyzeBranchDefault(prompt: string) -> RoleOutput {
  client AnthropicBalanced
  prompt #"
    {{ prompt }}
  "#
}
