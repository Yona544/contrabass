package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/junhoyeo/contrabass/internal/config"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a WORKFLOW.md for this repository",
		Long: `Scaffold a WORKFLOW.md with a chosen tracker and agent. Missing choices are
prompted interactively; pass --tracker/--agent/--model to skip the prompts.`,
		RunE: runInit,
	}
	cmd.Flags().String("output", "WORKFLOW.md", "where to write the workflow file")
	cmd.Flags().String("tracker", "", "tracker type (internal, linear, github, jira)")
	cmd.Flags().String("agent", "", "agent type (codex, claude, opencode, omx, omc)")
	cmd.Flags().String("model", "", "model name recorded in the workflow")
	cmd.Flags().Bool("force", false, "overwrite an existing workflow file")
	return cmd
}

func runInit(cmd *cobra.Command, _ []string) error {
	output, _ := cmd.Flags().GetString("output")
	trackerType, _ := cmd.Flags().GetString("tracker")
	agentType, _ := cmd.Flags().GetString("agent")
	model, _ := cmd.Flags().GetString("model")
	force, _ := cmd.Flags().GetBool("force")

	if !force {
		if _, err := os.Stat(output); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", output)
		}
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	trackerType = strings.TrimSpace(strings.ToLower(trackerType))
	if trackerType == "" {
		trackerType = prompt(reader, out, "Tracker (internal, linear, github, jira)", "internal")
	}
	if !knownTrackerTypes[trackerType] || trackerType == "local" {
		return fmt.Errorf("unknown tracker type %q (supported: internal, linear, github, jira)", trackerType)
	}

	agentType = strings.TrimSpace(strings.ToLower(agentType))
	if agentType == "" {
		agentType = prompt(reader, out, "Agent (claude, codex, opencode, omx, omc)", "claude")
	}
	if !knownAgentTypes[agentType] {
		return fmt.Errorf("unknown agent type %q (supported: codex, claude, opencode, omx, omc, oh-my-opencode)", agentType)
	}

	model = strings.TrimSpace(model)
	if model == "" {
		model = prompt(reader, out, "Model name", defaultModelForAgent(agentType))
	}

	content := renderWorkflowScaffold(trackerType, agentType, model)
	if err := os.WriteFile(output, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", output, err)
	}

	// Round-trip the scaffold through the real parser so init can never
	// produce a file contrabass itself rejects.
	if _, err := config.ParseWorkflow(output); err != nil {
		return fmt.Errorf("generated workflow failed validation (please report this bug): %w", err)
	}

	fmt.Fprintf(out, "Wrote %s (tracker: %s, agent: %s)\n", output, trackerType, agentType)
	fmt.Fprintf(out, "\nNext steps:\n")
	fmt.Fprintf(out, "  1. Review and edit %s\n", output)
	fmt.Fprintf(out, "  2. contrabass validate --config %s\n", output)
	fmt.Fprintf(out, "  3. contrabass doctor --config %s\n", output)
	fmt.Fprintf(out, "  4. contrabass --config %s\n", output)
	return nil
}

func prompt(reader *bufio.Reader, out interface{ Write([]byte) (int, error) }, label, defaultValue string) string {
	fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
	line, err := reader.ReadString('\n')
	value := strings.TrimSpace(line)
	if err != nil && value == "" {
		return defaultValue
	}
	if value == "" {
		return defaultValue
	}
	return strings.ToLower(value)
}

func defaultModelForAgent(agentType string) string {
	switch agentType {
	case "claude", "omc":
		return "claude-sonnet-4-6"
	case "codex", "omx":
		return "codex-mini"
	default:
		return "claude-sonnet-4-6"
	}
}

func renderWorkflowScaffold(trackerType, agentType, model string) string {
	var b strings.Builder

	b.WriteString("---\n")
	fmt.Fprintf(&b, "model: %s\n", model)
	b.WriteString("max_concurrency: 2\n")
	b.WriteString("poll_interval_ms: 5000\n")
	b.WriteString("agent_timeout_ms: 600000\n")
	b.WriteString("stall_timeout_ms: 120000\n")

	b.WriteString("tracker:\n")
	fmt.Fprintf(&b, "  type: %s\n", trackerType)
	switch trackerType {
	case "linear":
		b.WriteString("  project_url: https://linear.app/your-team/project/your-project\n")
		b.WriteString("  # Set LINEAR_API_KEY in the environment.\n")
	case "github":
		b.WriteString("  owner: your-org\n")
		b.WriteString("  repo: your-repo\n")
		b.WriteString("  # Set GITHUB_TOKEN in the environment.\n")
	case "internal":
		b.WriteString("  # board_dir: .contrabass/board\n")
	}

	if trackerType == "jira" {
		b.WriteString("jira:\n")
		b.WriteString("  base_url: https://your-team.atlassian.net\n")
		b.WriteString("  project: ABC\n")
		b.WriteString("  # Set JIRA_EMAIL and JIRA_API_TOKEN in the environment.\n")
	}

	b.WriteString("agent:\n")
	fmt.Fprintf(&b, "  type: %s\n", agentType)
	switch agentType {
	case "claude":
		b.WriteString("claude:\n")
		b.WriteString("  binary_path: claude\n")
		fmt.Fprintf(&b, "  model: %s\n", model)
		b.WriteString("  # permission_mode: bypassPermissions\n")
		b.WriteString("  # max_turns: 50\n")
	case "codex":
		b.WriteString("codex:\n")
		b.WriteString("  binary_path: codex app-server\n")
	case "opencode":
		b.WriteString("opencode:\n")
		b.WriteString("  binary_path: opencode serve\n")
	}

	b.WriteString(`
# --- Optional features (uncomment to enable) ---
# web:
#   listen: 0.0.0.0:8080          # team-shared dashboard; requires auth_token
#   auth_token: $CONTRABASS_DASHBOARD_TOKEN
# notifications:
#   slack_webhook_url: $SLACK_WEBHOOK_URL
# pull_request:
#   enabled: true                 # push verified runs and open a draft PR (gh CLI)
# schedule:
#   windows: ["22:00-06:00"]
#   days: [fri, sat]
#   max_issues: 10
`)
	b.WriteString("---\n")

	b.WriteString(`# Workflow prompt

You are working on this repository.
Working directory: {{ workspace.path }}

## Task

Issue: {{ issue.title }}
{{ issue.description }}

## Constraints

- Make minimal, focused changes
- Run the project's tests before declaring success
`)

	return b.String()
}
