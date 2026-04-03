class DocgenOutput {
  package_doc  string   @description("Package-level GoDoc comment (// Package ...)")
  symbol_docs  string[] @description("GoDoc comments for each exported symbol, one entry per symbol")
}

function GenerateDocs(file_content: string, file_path: string) -> DocgenOutput {
  client AnthropicBalanced
  prompt #"
    You are a Go documentation expert. Generate GoDoc-style documentation for this file.

    File: {{ file_path }}

    ```go
    {{ file_content }}
    ```

    - package_doc: the // Package ... comment for the top of the file
    - symbol_docs: one GoDoc comment per exported type, function, method, or constant

    {{ ctx.output_format }}
  "#
}
