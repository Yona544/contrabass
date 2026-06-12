package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/agent"
	"github.com/junhoyeo/contrabass/internal/config"
	"github.com/junhoyeo/contrabass/internal/tracker"
	"github.com/junhoyeo/contrabass/internal/types"
)

func oneShotTestConfig(t *testing.T, boardDir string) *config.WorkflowConfig {
	t.Helper()

	base, err := loadBaseConfig("", "claude")
	require.NoError(t, err)
	cfg := prepareSingleRunConfig(base, boardDir)
	// Keep test artifacts inside the temp dir.
	enabled := false
	cfg.History.Enabled = &enabled
	return cfg
}

func createOneShotIssue(t *testing.T, ctx context.Context, boardDir, title string) tracker.LocalBoardIssue {
	t.Helper()

	lt := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    boardDir,
		IssuePrefix: oneShotIssuePrefix,
		Actor:       "oneshot",
	})
	issue, err := lt.CreateIssueWithOptions(ctx, tracker.LocalIssueCreateOptions{
		Title:       title,
		Description: "do the thing",
		Labels:      []string{"oneshot"},
	})
	require.NoError(t, err)
	return issue
}

func TestRunSingleIssueSucceedsWithMockRunner(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	boardDir := tmp + "/board"
	cfg := oneShotTestConfig(t, boardDir)
	issue := createOneShotIssue(t, ctx, boardDir, "mock success")

	// Delay spaces the final event from the done signal so watchProcess
	// records turn/completed before completion — the established MockRunner
	// pattern in the orchestrator tests.
	runner := &agent.MockRunner{
		Events: []types.AgentEvent{
			{Type: "session/started"},
			{Type: "turn/completed"},
		},
		Delay: 10 * time.Millisecond,
	}

	out := &bytes.Buffer{}
	lt := tracker.NewLocalTracker(tracker.LocalConfig{BoardDir: boardDir, IssuePrefix: oneShotIssuePrefix})
	result, err := runSingleIssue(ctx, out, cfg, lt, issue.ID, runner, log.Default())
	require.NoError(t, err)

	assert.True(t, result.Succeeded, "phase=%s error=%s", result.Phase, result.Error)
	assert.Equal(t, "Succeeded", result.Phase)
	assert.Contains(t, out.String(), "agent started")
	assert.Contains(t, out.String(), "agent finished")

	done, err := lt.GetIssue(ctx, issue.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateDone, done.State)
}

func TestRunSingleIssueFailureSetsError(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	boardDir := tmp + "/board"
	cfg := oneShotTestConfig(t, boardDir)
	issue := createOneShotIssue(t, ctx, boardDir, "mock failure")

	runner := &agent.MockRunner{
		Events:  []types.AgentEvent{{Type: "session/started"}},
		DoneErr: assert.AnError,
		Delay:   10 * time.Millisecond,
	}

	out := &bytes.Buffer{}
	lt := tracker.NewLocalTracker(tracker.LocalConfig{BoardDir: boardDir, IssuePrefix: oneShotIssuePrefix})
	result, err := runSingleIssue(ctx, out, cfg, lt, issue.ID, runner, log.Default())
	require.NoError(t, err)

	assert.False(t, result.Succeeded)
	assert.NotEmpty(t, result.Error)
}

func TestPrepareSingleRunConfigForcesIsolation(t *testing.T) {
	base := &config.WorkflowConfig{}
	base.Tracker = config.TrackerConfig{Type: "linear", ProjectURL: "https://linear.app/x"}
	base.Schedule = config.ScheduleConfig{Windows: []string{"22:00-06:00"}}
	base.MaxConcurrencyRaw = 8

	cfg := prepareSingleRunConfig(base, "some/board")

	assert.Equal(t, "internal", cfg.TrackerType())
	assert.Equal(t, "some/board", cfg.LocalBoardDir())
	assert.Equal(t, config.TeamExecutionModeSingle, cfg.TeamExecutionMode())
	assert.Equal(t, 1, cfg.MaxConcurrency())
	assert.False(t, cfg.ScheduleEnabled(), "one-shots ignore schedule windows")
	assert.Equal(t, "{{ issue.description }}", cfg.PromptTemplate)
	// Base config untouched.
	assert.Equal(t, "linear", base.TrackerType())
}

func TestOneShotTitle(t *testing.T) {
	assert.Equal(t, "explicit", oneShotTitle("prompt", "explicit"))
	assert.Equal(t, "first line", oneShotTitle("first line\nsecond line", ""))
	long := string(bytes.Repeat([]byte("x"), 100))
	assert.Len(t, oneShotTitle(long, ""), 72)
}

func TestLoadBaseConfigSynthesizesDefault(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, err := loadBaseConfig("", "")
	require.NoError(t, err)
	assert.Equal(t, "claude", cfg.AgentType())
	assert.NotEmpty(t, cfg.ModelRaw)
	assert.Equal(t, "{{ issue.description }}", cfg.PromptTemplate)
}
