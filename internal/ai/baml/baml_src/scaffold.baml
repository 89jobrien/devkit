class ScaffoldOutput {
  file_content string @description("Complete Go source file following hexagonal architecture conventions")
  usage_notes  string @description("How to use the generated package — flag the injected runner pattern")
}

function GenerateScaffold(package_name: string, purpose: string, repo_context: string) -> ScaffoldOutput {
  client AnthropicBalanced
  prompt #"
    You are a Go architect. Generate a new Go package following hexagonal architecture.

    Package name: {{ package_name }}
    Purpose: {{ purpose }}

    Required elements:
    - Runner interface with Run(ctx context.Context, prompt string, tools []string) (string, error)
    - RunnerFunc adapter implementing Runner
    - Config struct with a Runner field
    - Run(ctx, cfg) function that validates cfg.Runner != nil then calls cfg.Runner.Run(ctx, buildPrompt(cfg), nil)
    - buildPrompt(cfg) function

    Repo conventions:
    {{ repo_context }}

    {{ ctx.output_format }}
  "#
}
