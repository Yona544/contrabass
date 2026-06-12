package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/config"
)

func runValidateCmd(t *testing.T, configBody string) (string, error) {
	t.Helper()

	cfgPath := filepath.Join(t.TempDir(), "WORKFLOW.md")
	require.NoError(t, os.WriteFile(cfgPath, []byte(configBody), 0o644))

	cmd := newValidateCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--config", cfgPath})
	err := cmd.Execute()
	return out.String(), err
}

func TestValidatePassesMinimalInternalConfig(t *testing.T) {
	stdout, err := runValidateCmd(t, `---
model: codex-mini
tracker:
  type: internal
agent:
  type: codex
---
Prompt.
`)
	require.NoError(t, err)
	assert.Contains(t, stdout, "PASS")
}

func TestValidateReportsUnknownAgentAndTracker(t *testing.T) {
	stdout, err := runValidateCmd(t, `---
model: m
tracker:
  type: fancytracker
agent:
  type: hal9000
---
Prompt.
`)
	require.Error(t, err)
	assert.Contains(t, stdout, `unknown tracker.type "fancytracker"`)
	assert.Contains(t, stdout, `unknown agent.type "hal9000"`)
}

func TestValidateRequiresModel(t *testing.T) {
	stdout, err := runValidateCmd(t, `---
tracker:
  type: internal
---
Prompt.
`)
	require.Error(t, err)
	assert.Contains(t, stdout, "model is required")
}

func TestValidateJiraRequirements(t *testing.T) {
	stdout, err := runValidateCmd(t, `---
model: m
tracker:
  type: jira
---
Prompt.
`)
	require.Error(t, err)
	assert.Contains(t, stdout, "jira.base_url")
	assert.Contains(t, stdout, "jira.project or jira.jql")
}

func TestValidateBadScheduleWindow(t *testing.T) {
	stdout, err := runValidateCmd(t, `---
model: m
tracker:
  type: internal
schedule:
  windows: ["25:00-26:00"]
---
Prompt.
`)
	require.Error(t, err)
	assert.Contains(t, stdout, "invalid schedule window")
}

func TestValidateNonLoopbackListenWithoutToken(t *testing.T) {
	t.Setenv("CONTRABASS_DASHBOARD_TOKEN", "")
	stdout, err := runValidateCmd(t, `---
model: m
tracker:
  type: internal
web:
  listen: 0.0.0.0:8080
---
Prompt.
`)
	require.Error(t, err)
	assert.Contains(t, stdout, "non-loopback")
}

func TestValidateBadWebhookURL(t *testing.T) {
	stdout, err := runValidateCmd(t, `---
model: m
tracker:
  type: internal
notifications:
  slack_webhook_url: not-a-url
---
Prompt.
`)
	require.Error(t, err)
	assert.Contains(t, stdout, "not a valid http(s) URL")
}

func TestValidateWorkflowConfigWarnsMissingCreds(t *testing.T) {
	cfg := &config.WorkflowConfig{
		ModelRaw: "m",
		Tracker:  config.TrackerConfig{Type: "linear"},
	}
	problems, warnings := validateWorkflowConfig(cfg)
	assert.Empty(t, problems)
	require.NotEmpty(t, warnings)
	assert.Contains(t, warnings[0], "LINEAR_API_KEY")
}
