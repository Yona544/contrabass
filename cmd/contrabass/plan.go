package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/junhoyeo/contrabass/internal/config"
	"github.com/junhoyeo/contrabass/internal/tracker"
)

const (
	// defaultPlanMaxIssues caps how many issues a single plan may create.
	defaultPlanMaxIssues = 8
	// planDescriptionPreviewLimit bounds the description excerpt shown in
	// the preview tree.
	planDescriptionPreviewLimit = 80
)

// planPromptTemplate is the instruction sent to the claude CLI on stdin.
// The verbs are deliberately strict: the command parses the reply as JSON.
const planPromptTemplate = `You are planning work for autonomous coding agents. Decompose the task below into 2 to %d issues for an internal issue board.

Respond with ONLY a JSON array, no prose and no markdown fences, shaped exactly like:
[{"title": string, "description": string, "blocked_by": [int, ...]}, ...]

Rules:
- "title" is a short imperative summary of the issue.
- "description" must be self-contained enough for a coding agent to execute the issue without reading the other issues: include context, affected files or components when known, and concrete acceptance criteria.
- "blocked_by" lists the zero-based array indices of issues that must be completed first. Indices may only reference entries EARLIER in the array. Use [] when the issue has no prerequisites.
- Order the array so prerequisites come before dependents.

Task:
%s
`

// planExecutor runs the claude CLI and returns its stdout. Tests replace it
// so they never invoke a real binary (mirrors doctorRunCommand).
var planExecutor = func(ctx context.Context, name string, args []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return stdout.String(), fmt.Errorf("%w: %s", err, detail)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

// planIssueSpec is one entry of the model's decomposition array.
type planIssueSpec struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	BlockedBy   []int  `json:"blocked_by"`
}

// planClaudeEnvelope is the shape of "claude -p --output-format json": the
// model's text lives in the result field.
type planClaudeEnvelope struct {
	Result string `json:"result"`
}

func newPlanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan <task description>",
		Short: "Decompose a task into internal-board issues with dependencies",
		Long: `Ask the claude CLI to decompose a task description into internal-board
issues wired together with blocked-by dependencies. Without --apply this only
prints the proposed plan; with --apply the issues are created in order so the
orchestrator's BlockedBy gating runs them in dependency order.`,
		Args: cobra.MinimumNArgs(1),
		RunE: runPlan,
	}
	cmd.Flags().String("config", "", "workflow file for claude binary/model defaults (default: WORKFLOW.md when present)")
	cmd.Flags().String("board-dir", "", "override internal board directory")
	cmd.Flags().Bool("apply", false, "create the issues instead of printing a preview")
	cmd.Flags().Int("max-issues", defaultPlanMaxIssues, "maximum number of issues the plan may contain")
	cmd.Flags().String("label", "planned", "label applied to created issues")
	return cmd
}

func runPlan(cmd *cobra.Command, args []string) error {
	task := strings.TrimSpace(strings.Join(args, " "))
	if task == "" {
		return fmt.Errorf("task description is required")
	}

	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return fmt.Errorf("getting config flag: %w", err)
	}
	boardDir, err := cmd.Flags().GetString("board-dir")
	if err != nil {
		return fmt.Errorf("getting board-dir flag: %w", err)
	}
	apply, err := cmd.Flags().GetBool("apply")
	if err != nil {
		return fmt.Errorf("getting apply flag: %w", err)
	}
	maxIssues, err := cmd.Flags().GetInt("max-issues")
	if err != nil {
		return fmt.Errorf("getting max-issues flag: %w", err)
	}
	label, err := cmd.Flags().GetString("label")
	if err != nil {
		return fmt.Errorf("getting label flag: %w", err)
	}
	if maxIssues < 1 {
		return fmt.Errorf("--max-issues must be at least 1")
	}

	cfg, err := loadBaseConfig(configPath, "claude")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if boardDir == "" {
		boardDir = cfg.LocalBoardDir()
	}

	issues, err := requestPlan(cmd.Context(), cfg, task, maxIssues)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if !apply {
		printPlanPreview(out, task, issues)
		return nil
	}

	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    boardDir,
		IssuePrefix: cfg.LocalIssuePrefix(),
		Actor:       "plan",
	})
	return applyPlan(cmd.Context(), out, localTracker, issues, label)
}

// requestPlan makes the single claude CLI call and returns the validated
// decomposition.
func requestPlan(ctx context.Context, cfg *config.WorkflowConfig, task string, maxIssues int) ([]planIssueSpec, error) {
	// Binary paths may carry embedded args (e.g. "bunx claude"); the first
	// field is the executable, the rest become leading args.
	fields := strings.Fields(cfg.ClaudeBinaryPath())
	name := fields[0]
	cliArgs := append(fields[1:], "-p", "--output-format", "json")
	if model := strings.TrimSpace(cfg.ClaudeModel()); model != "" {
		cliArgs = append(cliArgs, "--model", model)
	}

	prompt := fmt.Sprintf(planPromptTemplate, maxIssues, task)
	output, err := planExecutor(ctx, name, cliArgs, prompt)
	if err != nil {
		return nil, fmt.Errorf("running %s: %w", name, err)
	}

	issues, err := parsePlanOutput(output)
	if err != nil {
		return nil, err
	}
	if err := validatePlanIssues(issues, maxIssues); err != nil {
		return nil, err
	}
	return issues, nil
}

// parsePlanOutput unwraps the claude json envelope (its result field holds
// the model text), then extracts and decodes the JSON array. Bare arrays and
// fenced arrays are accepted for resilience.
func parsePlanOutput(output string) ([]planIssueSpec, error) {
	text := strings.TrimSpace(output)
	if text == "" {
		return nil, fmt.Errorf("claude produced no output")
	}

	if strings.HasPrefix(text, "{") {
		var envelope planClaudeEnvelope
		if err := json.Unmarshal([]byte(text), &envelope); err != nil {
			return nil, fmt.Errorf("parsing claude output envelope: %w", err)
		}
		if strings.TrimSpace(envelope.Result) == "" {
			return nil, fmt.Errorf("claude output envelope has an empty result field")
		}
		text = envelope.Result
	}

	arrayText, err := extractPlanArray(text)
	if err != nil {
		return nil, err
	}

	var issues []planIssueSpec
	if err := json.Unmarshal([]byte(arrayText), &issues); err != nil {
		return nil, fmt.Errorf("parsing plan JSON array: %w", err)
	}
	return issues, nil
}

// extractPlanArray locates the JSON array inside model text, stripping a
// markdown code fence (```json ... ```) when present.
func extractPlanArray(text string) (string, error) {
	text = strings.TrimSpace(text)
	if open := strings.Index(text, "```"); open >= 0 {
		rest := text[open+3:]
		// Drop the fence info string ("json") up to the first newline.
		if newline := strings.Index(rest, "\n"); newline >= 0 {
			rest = rest[newline+1:]
		}
		if closing := strings.Index(rest, "```"); closing >= 0 {
			rest = rest[:closing]
		}
		text = rest
	}

	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end <= start {
		return "", fmt.Errorf("claude output does not contain a JSON array")
	}
	return text[start : end+1], nil
}

// validatePlanIssues enforces the structural contract: a bounded number of
// titled issues whose blocked_by indices only point at earlier entries, so
// the dependency graph is acyclic by construction.
func validatePlanIssues(issues []planIssueSpec, maxIssues int) error {
	if len(issues) == 0 {
		return fmt.Errorf("plan contains no issues")
	}
	if len(issues) > maxIssues {
		return fmt.Errorf("plan contains %d issues, exceeding --max-issues %d", len(issues), maxIssues)
	}
	for i, issue := range issues {
		if strings.TrimSpace(issue.Title) == "" {
			return fmt.Errorf("plan issue %d has an empty title", i+1)
		}
		for _, dep := range issue.BlockedBy {
			if dep < 0 || dep >= i {
				return fmt.Errorf("plan issue %d (%q) has invalid blocked_by index %d: indices must reference earlier issues in the array", i+1, issue.Title, dep)
			}
		}
	}
	return nil
}

func printPlanPreview(out io.Writer, task string, issues []planIssueSpec) {
	taskLine := strings.TrimSpace(strings.SplitN(task, "\n", 2)[0])
	fmt.Fprintf(out, "plan for: %s\n\n", truncateScanText(taskLine, planDescriptionPreviewLimit))
	for i, issue := range issues {
		fmt.Fprintf(out, "%d. %s%s\n", i+1, strings.TrimSpace(issue.Title), planBlockedByNote(issue.BlockedBy))
		if first := planFirstLine(issue.Description); first != "" {
			fmt.Fprintf(out, "   %s\n", truncateScanText(first, planDescriptionPreviewLimit))
		}
	}
	fmt.Fprintf(out, "\n%d issue(s) planned; run again with --apply to create them\n", len(issues))
}

// planBlockedByNote renders zero-based blocked_by indices as the one-based
// "#n" numbers used in the preview tree.
func planBlockedByNote(deps []int) string {
	if len(deps) == 0 {
		return ""
	}
	refs := make([]string, 0, len(deps))
	for _, dep := range deps {
		refs = append(refs, fmt.Sprintf("#%d", dep+1))
	}
	return fmt.Sprintf(" (blocked by: %s)", strings.Join(refs, ", "))
}

func planFirstLine(description string) string {
	return strings.TrimSpace(strings.SplitN(strings.TrimSpace(description), "\n", 2)[0])
}

// applyPlan creates the issues in array order, translating blocked_by
// indices into the IDs of the issues already created.
func applyPlan(ctx context.Context, out io.Writer, localTracker *tracker.LocalTracker, issues []planIssueSpec, label string) error {
	var labels []string
	if trimmed := strings.TrimSpace(label); trimmed != "" {
		labels = []string{trimmed}
	}

	createdIDs := make([]string, 0, len(issues))
	for i, spec := range issues {
		var blockedBy []string
		for _, dep := range spec.BlockedBy {
			blockedBy = append(blockedBy, createdIDs[dep])
		}

		issue, err := localTracker.CreateIssueWithOptions(ctx, tracker.LocalIssueCreateOptions{
			Title:       strings.TrimSpace(spec.Title),
			Description: spec.Description,
			Labels:      labels,
			BlockedBy:   blockedBy,
		})
		if err != nil {
			return fmt.Errorf("creating issue %d (%q): %w", i+1, spec.Title, err)
		}
		createdIDs = append(createdIDs, issue.ID)

		if len(blockedBy) > 0 {
			fmt.Fprintf(out, "created %s %s (blocked by %s)\n", issue.ID, issue.Title, strings.Join(blockedBy, ", "))
		} else {
			fmt.Fprintf(out, "created %s %s\n", issue.ID, issue.Title)
		}
	}
	fmt.Fprintf(out, "created %d issue(s)\n", len(issues))
	return nil
}
