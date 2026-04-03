// internal/baml/adapter_tools.go — BAML adapter functions for the 7 new
// devkit subcommands: adr, docgen, migrate, scaffold, log-pattern, incident, profile.
//
// Each exported Run* function is a BAML-backed implementation that the
// corresponding internal package can use as its runner.
package baml

import (
	"context"
	"fmt"
	"strings"

	baml_client "baml_devkit"
	"baml_devkit/stream_types"
	"baml_devkit/types"
)

// RunADR drafts an ADR using BAML structured output.
func RunADR(ctx context.Context, title, ctxText string) (string, error) {
	ch, err := baml_client.Stream.DraftADR(ctx, title, ctxText)
	if err != nil {
		return "", fmt.Errorf("baml adr: %w", err)
	}
	return drainADR(ch)
}

// RunDocgen generates Go documentation using BAML structured output.
func RunDocgen(ctx context.Context, fileContent, filePath string) (string, error) {
	ch, err := baml_client.Stream.GenerateDocs(ctx, fileContent, filePath)
	if err != nil {
		return "", fmt.Errorf("baml docgen: %w", err)
	}
	return drainDocgen(ch)
}

// RunMigrate analyzes a breaking API change using BAML structured output.
func RunMigrate(ctx context.Context, oldAPI, newAPI, code, filePath string) (string, error) {
	ch, err := baml_client.Stream.AnalyzeMigration(ctx, oldAPI, newAPI, code, filePath)
	if err != nil {
		return "", fmt.Errorf("baml migrate: %w", err)
	}
	return drainMigrate(ch)
}

// RunScaffold generates hexagonal package boilerplate using BAML structured output.
func RunScaffold(ctx context.Context, packageName, purpose, repoContext string) (string, error) {
	ch, err := baml_client.Stream.GenerateScaffold(ctx, packageName, purpose, repoContext)
	if err != nil {
		return "", fmt.Errorf("baml scaffold: %w", err)
	}
	return drainScaffold(ch)
}

// RunLogPattern analyzes logs for recurring patterns using BAML structured output.
func RunLogPattern(ctx context.Context, logs string) (string, error) {
	ch, err := baml_client.Stream.AnalyzeLogPatterns(ctx, logs)
	if err != nil {
		return "", fmt.Errorf("baml log-pattern: %w", err)
	}
	return drainLogPattern(ch)
}

// RunIncident drafts a structured incident report using BAML structured output.
func RunIncident(ctx context.Context, description, logs string) (string, error) {
	ch, err := baml_client.Stream.DraftIncidentReport(ctx, description, logs)
	if err != nil {
		return "", fmt.Errorf("baml incident: %w", err)
	}
	return drainIncident(ch)
}

// RunProfile analyzes pprof/benchmark output using BAML structured output.
func RunProfile(ctx context.Context, input string) (string, error) {
	ch, err := baml_client.Stream.AnalyzeProfile(ctx, input)
	if err != nil {
		return "", fmt.Errorf("baml profile: %w", err)
	}
	return drainProfile(ch)
}

// ─── ADR ────────────────────────────────────────────────────────────────────

func drainADR(ch <-chan baml_client.StreamValue[stream_types.ADROutput, types.ADROutput]) (string, error) {
	var final *types.ADROutput
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML adr")
	}
	return formatADR(final), nil
}

func formatADR(r *types.ADROutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Status\n%s\n\n", r.Status)
	fmt.Fprintf(&sb, "## Context\n%s\n\n", r.Context)
	fmt.Fprintf(&sb, "## Decision\n%s\n\n", r.Decision)
	if len(r.Consequences) > 0 {
		sb.WriteString("## Consequences\n")
		for _, c := range r.Consequences {
			fmt.Fprintf(&sb, "- %s\n", c)
		}
	}
	return sb.String()
}

// ─── DOCGEN ─────────────────────────────────────────────────────────────────

func drainDocgen(ch <-chan baml_client.StreamValue[stream_types.DocgenOutput, types.DocgenOutput]) (string, error) {
	var final *types.DocgenOutput
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML docgen")
	}
	return formatDocgen(final), nil
}

func formatDocgen(r *types.DocgenOutput) string {
	var sb strings.Builder
	sb.WriteString(r.Package_doc)
	sb.WriteString("\n")
	for _, doc := range r.Symbol_docs {
		sb.WriteString("\n")
		sb.WriteString(doc)
		sb.WriteString("\n")
	}
	return sb.String()
}

// ─── MIGRATE ────────────────────────────────────────────────────────────────

func drainMigrate(ch <-chan baml_client.StreamValue[stream_types.MigrateOutput, types.MigrateOutput]) (string, error) {
	var final *types.MigrateOutput
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML migrate")
	}
	return formatMigrate(final), nil
}

func formatMigrate(r *types.MigrateOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Summary:** %s\n\n", r.Summary)
	if r.Diff != "" {
		sb.WriteString("```diff\n")
		sb.WriteString(r.Diff)
		sb.WriteString("\n```\n\n")
	}
	if len(r.Notes) > 0 {
		sb.WriteString("**Notes:**\n")
		for _, n := range r.Notes {
			fmt.Fprintf(&sb, "- %s\n", n)
		}
	}
	return sb.String()
}

// ─── SCAFFOLD ───────────────────────────────────────────────────────────────

func drainScaffold(ch <-chan baml_client.StreamValue[stream_types.ScaffoldOutput, types.ScaffoldOutput]) (string, error) {
	var final *types.ScaffoldOutput
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML scaffold")
	}
	return formatScaffold(final), nil
}

func formatScaffold(r *types.ScaffoldOutput) string {
	var sb strings.Builder
	sb.WriteString(r.File_content)
	if r.Usage_notes != "" {
		sb.WriteString("\n\n// Usage notes:\n")
		for _, line := range strings.Split(r.Usage_notes, "\n") {
			fmt.Fprintf(&sb, "// %s\n", line)
		}
	}
	return sb.String()
}

// ─── LOG PATTERN ────────────────────────────────────────────────────────────

func drainLogPattern(ch <-chan baml_client.StreamValue[stream_types.LogPatternOutput, types.LogPatternOutput]) (string, error) {
	var final *types.LogPatternOutput
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML log-pattern")
	}
	return formatLogPattern(final), nil
}

func formatLogPattern(r *types.LogPatternOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Summary:** %s\n\n", r.Summary)
	if len(r.Patterns) > 0 {
		sb.WriteString("**Patterns:**\n\n")
		for _, p := range r.Patterns {
			fmt.Fprintf(&sb, "### [%s] %s (×%d)\n", p.Severity, p.Pattern, p.Count)
			fmt.Fprintf(&sb, "- First seen: %s\n", p.First_seen)
			fmt.Fprintf(&sb, "- Last seen: %s\n", p.Last_seen)
			fmt.Fprintf(&sb, "- Suggestion: %s\n\n", p.Suggestion)
		}
	}
	return sb.String()
}

// ─── INCIDENT ───────────────────────────────────────────────────────────────

func drainIncident(ch <-chan baml_client.StreamValue[stream_types.IncidentOutput, types.IncidentOutput]) (string, error) {
	var final *types.IncidentOutput
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML incident")
	}
	return formatIncident(final), nil
}

func formatIncident(r *types.IncidentOutput) string {
	var sb strings.Builder
	if len(r.Timeline) > 0 {
		sb.WriteString("## Timeline\n")
		for _, t := range r.Timeline {
			fmt.Fprintf(&sb, "- %s\n", t)
		}
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "## Root Cause\n%s\n\n", r.Root_cause)
	fmt.Fprintf(&sb, "## Impact\n%s\n\n", r.Impact)
	if len(r.Mitigations_applied) > 0 {
		sb.WriteString("## Mitigations Applied\n")
		for _, m := range r.Mitigations_applied {
			fmt.Fprintf(&sb, "- %s\n", m)
		}
		sb.WriteString("\n")
	}
	if len(r.Follow_up_actions) > 0 {
		sb.WriteString("## Follow-up Actions\n")
		for i, a := range r.Follow_up_actions {
			fmt.Fprintf(&sb, "%d. %s — **%s** (due: %s)\n", i+1, a.Action, a.Owner, a.Due)
		}
	}
	return sb.String()
}

// ─── PROFILE ────────────────────────────────────────────────────────────────

func drainProfile(ch <-chan baml_client.StreamValue[stream_types.ProfileOutput, types.ProfileOutput]) (string, error) {
	var final *types.ProfileOutput
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML profile")
	}
	return formatProfile(final), nil
}

func formatProfile(r *types.ProfileOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Summary:** %s\n\n", r.Summary)
	if len(r.Hotspots) > 0 {
		sb.WriteString("**Hotspots:**\n\n")
		for _, h := range r.Hotspots {
			fmt.Fprintf(&sb, "### %s (%s)\n", h.Symbol, h.Cost)
			fmt.Fprintf(&sb, "%s\n\n**Suggestion:** %s\n\n", h.Explanation, h.Suggestion)
		}
	}
	if len(r.Quick_wins) > 0 {
		sb.WriteString("**Quick Wins:**\n")
		for _, w := range r.Quick_wins {
			fmt.Fprintf(&sb, "- %s\n", w)
		}
	}
	return sb.String()
}
