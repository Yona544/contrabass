package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/config"
	"github.com/junhoyeo/contrabass/internal/history"
	"github.com/junhoyeo/contrabass/internal/tracker"
)

func TestPrepareResumeConfigClaudeSession(t *testing.T) {
	base := &config.WorkflowConfig{}
	base.Agent.Type = "claude"
	base.PromptTemplate = "{{ issue.description }}"

	record := &history.Record{AgentType: "claude", SessionID: "sess-123"}
	cfg := prepareResumeConfig(base, "board", record, "handle the nil case {{ sneaky }}")

	assert.Equal(t, []string{"--resume", "sess-123"}, cfg.Claude.ExtraArgs)
	assert.Contains(t, cfg.PromptTemplate, "Reviewer feedback")
	assert.Contains(t, cfg.PromptTemplate, "{% raw %}handle the nil case {{ sneaky }}{% endraw %}")
	assert.Empty(t, base.Claude.ExtraArgs, "base config untouched")
}

func TestPrepareResumeConfigClaudeSessionNoFeedback(t *testing.T) {
	base := &config.WorkflowConfig{}
	base.Agent.Type = "claude"
	base.PromptTemplate = "{{ issue.description }}"

	record := &history.Record{AgentType: "claude", SessionID: "sess-9"}
	cfg := prepareResumeConfig(base, "board", record, "")

	assert.Equal(t, []string{"--resume", "sess-9"}, cfg.Claude.ExtraArgs)
	assert.Contains(t, cfg.PromptTemplate, "Continue your previous work")
}

func TestPrepareResumeConfigNonResumableAppendsFeedback(t *testing.T) {
	base := &config.WorkflowConfig{}
	base.Agent.Type = "codex"
	base.PromptTemplate = "Task: {{ issue.title }}"

	cfg := prepareResumeConfig(base, "board", nil, "try again with tests")

	assert.Empty(t, cfg.Claude.ExtraArgs)
	assert.Contains(t, cfg.PromptTemplate, "Task: {{ issue.title }}")
	assert.Contains(t, cfg.PromptTemplate, "{% raw %}try again with tests{% endraw %}")
}

func TestPrepareResumeConfigUsesRecordAgentType(t *testing.T) {
	base := &config.WorkflowConfig{}
	base.Agent.Type = "codex"

	record := &history.Record{AgentType: "claude", SessionID: "s1"}
	cfg := prepareResumeConfig(base, "board", record, "")

	assert.Equal(t, "claude", cfg.AgentType())
	assert.Equal(t, []string{"--resume", "s1"}, cfg.Claude.ExtraArgs)
}

func TestFindBoardIssueTriesCaseVariants(t *testing.T) {
	tmp := t.TempDir()
	ctx := context.Background()
	lt := tracker.NewLocalTracker(tracker.LocalConfig{BoardDir: tmp, IssuePrefix: "RUN"})
	created, err := lt.CreateIssueWithOptions(ctx, tracker.LocalIssueCreateOptions{Title: "t", Description: "d"})
	require.NoError(t, err)

	cfg := &config.WorkflowConfig{}
	dir, issue, err := findBoardIssue(ctx, created.ID, tmp, cfg)
	require.NoError(t, err)
	assert.Equal(t, tmp, dir)
	assert.Equal(t, created.ID, issue.ID)

	// Lower-cased id (e.g. derived from a branch name) still resolves.
	_, issue, err = findBoardIssue(ctx, "run-1", tmp, cfg)
	require.NoError(t, err)
	assert.Equal(t, created.ID, issue.ID)
}

func TestFindBoardIssueNotFound(t *testing.T) {
	cfg := &config.WorkflowConfig{}
	_, _, err := findBoardIssue(context.Background(), "CB-404", t.TempDir(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CB-404 not found")
}

func TestLatestHistoryRecordPicksNewest(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.WorkflowConfig{History: config.HistoryConfig{Dir: tmp}}
	store := history.NewStore(tmp)

	base := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	require.NoError(t, store.Append(history.Record{IssueID: "RUN-1", SessionID: "old", FinishedAt: base}))
	require.NoError(t, store.Append(history.Record{IssueID: "RUN-1", SessionID: "new", FinishedAt: base.Add(time.Hour)}))
	require.NoError(t, store.Append(history.Record{IssueID: "RUN-2", SessionID: "other", FinishedAt: base.Add(2 * time.Hour)}))

	record, err := latestHistoryRecord(cfg, "run-1")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "new", record.SessionID)

	record, err = latestHistoryRecord(cfg, "RUN-404")
	require.NoError(t, err)
	assert.Nil(t, record)
}

func TestResumeFromReviewDerivesIssueIDFromBranch(t *testing.T) {
	t.Chdir(t.TempDir())

	err := resumeFromReview("symphony/run-7", "feedback")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RUN-7 not found")
}
