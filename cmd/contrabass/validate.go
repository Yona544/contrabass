package main

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/junhoyeo/contrabass/internal/config"
)

var validateLookPath = exec.LookPath

var knownAgentTypes = map[string]bool{
	"codex": true, "claude": true, "opencode": true,
	"omx": true, "omc": true, "oh-my-opencode": true,
}

var knownTrackerTypes = map[string]bool{
	"internal": true, "local": true, "linear": true, "github": true, "jira": true,
}

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a WORKFLOW.md configuration",
		Long:  "Parse and validate a WORKFLOW.md file, reporting configuration errors and likely misconfigurations without starting any runs.",
		RunE:  runValidate,
	}
	cmd.Flags().String("config", "", "path to WORKFLOW.md file (required)")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}

func runValidate(cmd *cobra.Command, _ []string) error {
	cfgPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return fmt.Errorf("getting config flag: %w", err)
	}

	cfg, err := config.ParseWorkflow(cfgPath)
	if err != nil {
		return fmt.Errorf("parsing workflow config: %w", err)
	}

	problems, warnings := validateWorkflowConfig(cfg)

	out := cmd.OutOrStdout()
	for _, warning := range warnings {
		fmt.Fprintf(out, "WARN  %s\n", warning)
	}
	for _, problem := range problems {
		fmt.Fprintf(out, "ERROR %s\n", problem)
	}

	if len(problems) > 0 {
		return fmt.Errorf("workflow config has %d error(s)", len(problems))
	}

	fmt.Fprintf(out, "PASS  %s is valid (%d warning(s))\n", cfgPath, len(warnings))
	return nil
}

// validateWorkflowConfig returns hard errors (config cannot work) and
// warnings (config likely needs attention, e.g. credentials expected from
// the environment at runtime).
func validateWorkflowConfig(cfg *config.WorkflowConfig) (problems []string, warnings []string) {
	if _, err := cfg.Model(); err != nil {
		problems = append(problems, "model is required (set model: in the front matter)")
	}

	trackerType := cfg.TrackerType()
	if !knownTrackerTypes[trackerType] {
		problems = append(problems, fmt.Sprintf("unknown tracker.type %q (supported: internal, local, linear, github, jira)", trackerType))
	}

	agentType := cfg.AgentType()
	if !knownAgentTypes[agentType] {
		problems = append(problems, fmt.Sprintf("unknown agent.type %q (supported: codex, claude, opencode, omx, omc, oh-my-opencode)", agentType))
	}

	if err := cfg.ValidateWorkerMode(); err != nil {
		problems = append(problems, err.Error())
	}

	if trackerType == "linear" {
		if err := cfg.ValidateLinearSyncCommentsMode(); err != nil && !errors.Is(err, config.ErrLinearSyncCommentsModeUnsupported) {
			problems = append(problems, err.Error())
		}
		if cfg.Tracker.Token == "" {
			warnings = append(warnings, "linear tracker has no tracker.token; LINEAR_API_KEY must be set at runtime")
		}
	}

	if trackerType == "github" {
		if cfg.GitHubOwner() == "" || cfg.GitHubRepo() == "" {
			warnings = append(warnings, "github tracker is missing tracker.owner/tracker.repo; GITHUB_OWNER/GITHUB_REPO must be set at runtime")
		}
	}

	if trackerType == "jira" {
		if strings.TrimSpace(cfg.Jira.BaseURL) == "" {
			problems = append(problems, "jira tracker requires jira.base_url (e.g. https://yourteam.atlassian.net)")
		}
		if strings.TrimSpace(cfg.Jira.Project) == "" && strings.TrimSpace(cfg.Jira.JQL) == "" {
			problems = append(problems, "jira tracker requires jira.project or jira.jql")
		}
		if strings.TrimSpace(cfg.Jira.Email) == "" || strings.TrimSpace(cfg.Jira.APIToken) == "" {
			warnings = append(warnings, "jira credentials not in config; JIRA_EMAIL and JIRA_API_TOKEN must be set at runtime")
		}
	}

	if _, err := buildScheduleGate(cfg); err != nil {
		problems = append(problems, err.Error())
	}

	if listen := cfg.WebListen(); listen != "" {
		if _, err := resolveWebOptions(cfg, "", 0); err != nil {
			problems = append(problems, err.Error())
		}
	}

	for _, endpoint := range []struct{ name, value string }{
		{"notifications.slack_webhook_url", cfg.Notifications.SlackWebhookURL},
		{"notifications.webhook_url", cfg.Notifications.WebhookURL},
	} {
		if endpoint.value == "" {
			continue
		}
		parsed, err := url.Parse(endpoint.value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			problems = append(problems, fmt.Sprintf("%s is not a valid http(s) URL", endpoint.name))
		}
	}

	if cfg.PullRequest.Enabled {
		if _, err := validateLookPath("gh"); err != nil {
			warnings = append(warnings, "pull_request.enabled is set but the gh CLI is not in PATH")
		}
	}

	if agentType == "claude" {
		if _, err := validateLookPath(strings.Fields(cfg.ClaudeBinaryPath())[0]); err != nil {
			warnings = append(warnings, fmt.Sprintf("agent.type claude but %q is not in PATH", cfg.ClaudeBinaryPath()))
		}
	}

	return problems, warnings
}
