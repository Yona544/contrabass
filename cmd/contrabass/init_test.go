package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/config"
)

func runInitCmd(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()

	cmd := newInitCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestInitScaffoldsParseableWorkflow(t *testing.T) {
	output := filepath.Join(t.TempDir(), "WORKFLOW.md")

	stdout, err := runInitCmd(t, "",
		"--output", output, "--tracker", "jira", "--agent", "claude", "--model", "claude-sonnet-4-6")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Wrote")

	cfg, err := config.ParseWorkflow(output)
	require.NoError(t, err)
	assert.Equal(t, "jira", cfg.TrackerType())
	assert.Equal(t, "claude", cfg.AgentType())
	model, err := cfg.Model()
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-4-6", model)
	assert.Equal(t, "https://your-team.atlassian.net", cfg.Jira.BaseURL)
}

func TestInitPromptsForMissingChoices(t *testing.T) {
	output := filepath.Join(t.TempDir(), "WORKFLOW.md")

	// Tracker entered, agent and model accepted as defaults (empty lines).
	stdout, err := runInitCmd(t, "github\n\n\n", "--output", output)
	require.NoError(t, err)
	assert.Contains(t, stdout, "tracker: github, agent: claude")

	cfg, err := config.ParseWorkflow(output)
	require.NoError(t, err)
	assert.Equal(t, "github", cfg.TrackerType())
	assert.Equal(t, "claude", cfg.AgentType())
}

func TestInitRefusesOverwriteWithoutForce(t *testing.T) {
	output := filepath.Join(t.TempDir(), "WORKFLOW.md")
	require.NoError(t, os.WriteFile(output, []byte("existing"), 0o644))

	_, err := runInitCmd(t, "", "--output", output, "--tracker", "internal", "--agent", "codex", "--model", "m")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	_, err = runInitCmd(t, "", "--output", output, "--tracker", "internal", "--agent", "codex", "--model", "m", "--force")
	require.NoError(t, err)
}

func TestInitRejectsUnknownChoices(t *testing.T) {
	output := filepath.Join(t.TempDir(), "WORKFLOW.md")

	_, err := runInitCmd(t, "", "--output", output, "--tracker", "fancytracker", "--agent", "claude", "--model", "m")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tracker type")

	_, err = runInitCmd(t, "", "--output", output, "--tracker", "internal", "--agent", "hal9000", "--model", "m")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent type")
}
