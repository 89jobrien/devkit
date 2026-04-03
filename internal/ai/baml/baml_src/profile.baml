class ProfileHotspot {
  symbol      string @description("Function, method, or allocation site")
  cost        string @description("CPU%, memory%, or benchmark ns/op as shown in input")
  explanation string @description("Plain English explanation of why this is expensive")
  suggestion  string @description("Specific optimization suggestion")
}

class ProfileOutput {
  summary   string          @description("2-3 sentence plain English summary of the profile")
  hotspots  ProfileHotspot[] @description("Top hotspots sorted by cost descending")
  quick_wins string[]       @description("Highest ROI optimizations to try first")
}

function AnalyzeProfile(input: string) -> ProfileOutput {
  client AnthropicBalanced
  prompt #"
    You are a Go performance expert. Analyze this pprof or benchmark output.

    ```
    {{ input }}
    ```

    Identify hotspots, explain them in plain English, and suggest specific optimizations.

    {{ ctx.output_format }}
  "#
}
