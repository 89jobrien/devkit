// Package baml provides a council.Runner adapter backed by BAML structured
// output with streaming token support.
package baml

import (
	"context"
	"fmt"
	"io"
	"strings"

	baml_client "baml_client/baml_client"
	"baml_client/baml_client/stream_types"
	"baml_client/baml_client/types"
)

// runFunc is the injectable call signature used by both the real client and
// test stubs.
type runFunc func(ctx context.Context, role, prompt string) (string, error)

// Adapter implements council.Runner using BAML structured output.
// Streaming partial tokens are written to out as they arrive.
type Adapter struct {
	role string
	out  io.Writer
	run  runFunc
}

// New returns an Adapter backed by the real generated BAML client.
// out is where streaming partial tokens are written (e.g. os.Stdout).
func New(role string, out io.Writer) *Adapter {
	return &Adapter{role: role, out: out, run: realRun}
}

// NewWithStub is used in tests to inject a stub run function.
func NewWithStub(role string, out io.Writer, fn runFunc) *Adapter {
	return &Adapter{role: role, out: out, run: fn}
}

// Run satisfies council.Runner. It calls the per-role BAML function, streams
// partial tokens to a.out, and returns the final result as a markdown string.
// The tools []string parameter is accepted for interface compliance but unused.
func (a *Adapter) Run(ctx context.Context, prompt string, _ []string) (string, error) {
	result, err := a.run(ctx, a.role, prompt)
	if err != nil {
		return "", fmt.Errorf("baml adapter [%s]: %w", a.role, err)
	}
	return result, nil
}

// realRun dispatches to the per-role BAML streaming function, drains the
// channel writing partials to stdout, and returns final formatted markdown.
func realRun(ctx context.Context, role, prompt string) (string, error) {
	switch role {
	case "strict-critic":
		ch, err := baml_client.Stream.AnalyzeBranchStrictCritic(ctx, prompt)
		if err != nil {
			return "", err
		}
		return drainStrictCritic(ch)
	case "creative-explorer":
		ch, err := baml_client.Stream.AnalyzeBranchCreativeExplorer(ctx, prompt)
		if err != nil {
			return "", err
		}
		return drainCreativeExplorer(ch)
	case "security-reviewer":
		ch, err := baml_client.Stream.AnalyzeBranchSecurityReviewer(ctx, prompt)
		if err != nil {
			return "", err
		}
		return drainSecurityReviewer(ch)
	case "general-analyst":
		ch, err := baml_client.Stream.AnalyzeBranchGeneralAnalyst(ctx, prompt)
		if err != nil {
			return "", err
		}
		return drainGeneralAnalyst(ch)
	case "pr":
		ch, err := baml_client.Stream.DraftPR(ctx, prompt)
		if err != nil {
			return "", err
		}
		return drainDraftPR(ch)
	default:
		ch, err := baml_client.Stream.AnalyzeBranchDefault(ctx, prompt)
		if err != nil {
			return "", err
		}
		return drainDefault(ch)
	}
}

func drainStrictCritic(ch <-chan baml_client.StreamValue[stream_types.StrictCriticOutput, types.StrictCriticOutput]) (string, error) {
	var final *types.StrictCriticOutput
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML strict-critic")
	}
	return formatStrictCritic(final), nil
}

func drainCreativeExplorer(ch <-chan baml_client.StreamValue[stream_types.CreativeExplorerOutput, types.CreativeExplorerOutput]) (string, error) {
	var final *types.CreativeExplorerOutput
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML creative-explorer")
	}
	return formatCreativeExplorer(final), nil
}

func drainSecurityReviewer(ch <-chan baml_client.StreamValue[stream_types.SecurityReviewerOutput, types.SecurityReviewerOutput]) (string, error) {
	var final *types.SecurityReviewerOutput
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML security-reviewer")
	}
	return formatSecurityReviewer(final), nil
}

func drainGeneralAnalyst(ch <-chan baml_client.StreamValue[stream_types.GeneralAnalystOutput, types.GeneralAnalystOutput]) (string, error) {
	var final *types.GeneralAnalystOutput
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML general-analyst")
	}
	return formatGeneralAnalyst(final), nil
}

func drainDraftPR(ch <-chan baml_client.StreamValue[stream_types.PRDescription, types.PRDescription]) (string, error) {
	var final *types.PRDescription
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML pr")
	}
	return formatPRDescription(final), nil
}

func drainDefault(ch <-chan baml_client.StreamValue[stream_types.RoleOutput, types.RoleOutput]) (string, error) {
	var final *types.RoleOutput
	for v := range ch {
		if v.IsError {
			return "", v.Error
		}
		if v.IsFinal {
			final = v.Final()
		}
	}
	if final == nil {
		return "", fmt.Errorf("no final value received from BAML default role")
	}
	return formatRoleOutput(final), nil
}

func formatStrictCritic(r *types.StrictCriticOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Health Score:** %.2f\n\n", r.Health_score)
	fmt.Fprintf(&sb, "**Summary:**\n%s\n\n", r.Summary)
	if len(r.Recommendations) > 0 {
		sb.WriteString("**Recommendations:**\n")
		for _, rec := range r.Recommendations {
			fmt.Fprintf(&sb, "- %s\n", rec)
		}
		sb.WriteString("\n")
	}
	if len(r.Risks) > 0 {
		sb.WriteString("**Risks:**\n")
		for _, risk := range r.Risks {
			fmt.Fprintf(&sb, "- %s\n", risk)
		}
	}
	return sb.String()
}

func formatCreativeExplorer(r *types.CreativeExplorerOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Health Score:** %.2f\n\n", r.Health_score)
	fmt.Fprintf(&sb, "**Summary:**\n%s\n\n", r.Summary)
	if len(r.Recommendations) > 0 {
		sb.WriteString("**Recommendations:**\n")
		for _, rec := range r.Recommendations {
			fmt.Fprintf(&sb, "- %s\n", rec)
		}
		sb.WriteString("\n")
	}
	if len(r.Innovation_opportunities) > 0 {
		sb.WriteString("**Innovation Opportunities:**\n")
		for _, opp := range r.Innovation_opportunities {
			fmt.Fprintf(&sb, "- %s\n", opp)
		}
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "**Architectural Potential:**\n%s\n", r.Architectural_potential)
	return sb.String()
}

func formatSecurityReviewer(r *types.SecurityReviewerOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Health Score:** %.2f\n\n", r.Health_score)
	fmt.Fprintf(&sb, "**Summary:**\n%s\n\n", r.Summary)
	if len(r.Recommendations) > 0 {
		sb.WriteString("**Recommendations:**\n")
		for _, rec := range r.Recommendations {
			fmt.Fprintf(&sb, "- %s\n", rec)
		}
		sb.WriteString("\n")
	}
	if len(r.Findings) > 0 {
		sb.WriteString("**Findings:**\n")
		for _, f := range r.Findings {
			fmt.Fprintf(&sb, "- [%s] %s\n", f.Severity, f.Description)
		}
	}
	return sb.String()
}

func formatGeneralAnalyst(r *types.GeneralAnalystOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Health Score:** %.2f\n\n", r.Health_score)
	fmt.Fprintf(&sb, "**Summary:**\n%s\n\n", r.Summary)
	if len(r.Recommendations) > 0 {
		sb.WriteString("**Recommendations:**\n")
		for _, rec := range r.Recommendations {
			fmt.Fprintf(&sb, "- %s\n", rec)
		}
		sb.WriteString("\n")
	}
	if len(r.Gaps) > 0 {
		sb.WriteString("**Gaps:**\n")
		for _, g := range r.Gaps {
			fmt.Fprintf(&sb, "- %s\n", g)
		}
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "**Work Patterns:**\n%s\n", r.Work_patterns)
	return sb.String()
}

func formatPRDescription(r *types.PRDescription) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", r.Title)
	fmt.Fprintf(&sb, "%s\n", r.Summary)
	if len(r.Changes) > 0 {
		sb.WriteString("\n## Changes\n")
		for _, c := range r.Changes {
			fmt.Fprintf(&sb, "- %s\n", c)
		}
	}
	if len(r.Test_plan) > 0 {
		sb.WriteString("\n## Test Plan\n")
		for _, t := range r.Test_plan {
			fmt.Fprintf(&sb, "- %s\n", t)
		}
	}
	return sb.String()
}

func formatRoleOutput(r *types.RoleOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Health Score:** %.2f\n\n", r.Health_score)
	fmt.Fprintf(&sb, "**Summary:**\n%s\n\n", r.Summary)
	if len(r.Recommendations) > 0 {
		sb.WriteString("**Recommendations:**\n")
		for _, rec := range r.Recommendations {
			fmt.Fprintf(&sb, "- %s\n", rec)
		}
	}
	return sb.String()
}
