class MigrateOutput {
  summary string   @description("One sentence: what changed and why callers must update")
  diff    string   @description("Unified diff of suggested callsite changes")
  notes   string[] @description("Migration notes, caveats, or edge cases")
}

function AnalyzeMigration(old_api: string, new_api: string, code: string, file_path: string) -> MigrateOutput {
  client AnthropicBalanced
  prompt #"
    You are a Go refactoring expert. Analyze this breaking API change and suggest callsite updates.

    Old API:
    ```
    {{ old_api }}
    ```

    New API:
    ```
    {{ new_api }}
    ```

    File: {{ file_path }}
    ```go
    {{ code }}
    ```

    Produce a unified diff in the diff field. Use --- a/{{ file_path }} and +++ b/{{ file_path }} headers.

    {{ ctx.output_format }}
  "#
}
