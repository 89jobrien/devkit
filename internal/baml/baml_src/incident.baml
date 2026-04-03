class IncidentAction {
  action    string @description("Specific follow-up action required")
  owner     string @description("Team or role responsible (infer from context or use 'TBD')")
  due       string @description("Due date or timeframe (infer or use 'TBD')")
}

class IncidentOutput {
  timeline           string[]       @description("Ordered timeline of events")
  root_cause         string         @description("Root cause analysis — specific, not vague")
  impact             string         @description("Scope of impact: affected systems, users, duration")
  mitigations_applied string[]      @description("Actions already taken to resolve the incident")
  follow_up_actions  IncidentAction[] @description("Required follow-up actions with owner and due date")
}

function DraftIncidentReport(description: string, logs: string) -> IncidentOutput {
  client AnthropicBalanced
  prompt #"
    You are an incident response engineer. Produce a structured incident report.

    Incident Description:
    {{ description }}

    {% if logs %}Supporting Logs:
    ```
    {{ logs }}
    ```
    {% endif %}

    Be specific. Infer owners and due dates where possible from context.

    {{ ctx.output_format }}
  "#
}
