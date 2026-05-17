package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/tracker"
	"github.com/junhoyeo/contrabass/internal/types"
)

func TestBoardCommandLifecycle(t *testing.T) {
	t.Parallel()

	boardDir := t.TempDir()

	run := func(args ...string) string {
		t.Helper()

		cmd := newRootCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs(args)
		require.NoError(t, cmd.Execute())
		return buf.String()
	}

	initOutput := run("board", "init", "--dir", boardDir, "--prefix", "OPS")
	assert.Contains(t, initOutput, "initialized board")
	assert.Contains(t, initOutput, "OPS")

	createOutput := run(
		"board", "create",
		"--dir", boardDir,
		"--title", "Ship local tracker",
		"--description", "Implement the first local board slice",
		"--labels", "tracker,local",
	)

	issueID := strings.TrimSpace(createOutput)
	require.Equal(t, "OPS-1", issueID)

	listOutput := run("board", "list", "--dir", boardDir)
	assert.Contains(t, listOutput, "OPS-1")
	assert.Contains(t, listOutput, "todo")
	assert.Contains(t, listOutput, "Ship local tracker")

	moveOutput := run("board", "move", "--dir", boardDir, issueID, "in_progress")
	assert.Contains(t, moveOutput, "OPS-1 -> in_progress")

	commentOutput := run("board", "comment", "--dir", boardDir, issueID, "--body", "Looks good")
	assert.Contains(t, commentOutput, "commented on OPS-1")

	showOutput := run("board", "show", "--dir", boardDir, issueID)
	assert.Contains(t, showOutput, "ID: OPS-1")
	assert.Contains(t, showOutput, "State: in_progress")
	assert.Contains(t, showOutput, "Looks good")
}

func TestBoardAssignCommandUpdatesAssignee(t *testing.T) {
	t.Parallel()

	boardDir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()

		cmd := newRootCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs(args)
		require.NoError(t, cmd.Execute())
		return buf.String()
	}

	run("board", "init", "--dir", boardDir, "--prefix", "OPS")
	issueID := strings.TrimSpace(run("board", "create", "--dir", boardDir, "--title", "Assign me"))
	require.Equal(t, "OPS-1", issueID)

	assignOutput := run("board", "assign", "--dir", boardDir, issueID, "worker-a")
	assert.Contains(t, assignOutput, "OPS-1 -> worker-a")

	showOutput := run("board", "show", "--dir", boardDir, issueID)
	assert.Contains(t, showOutput, "Assignee: worker-a")
}

func TestBoardCreateFlagsDoNotLeakAcrossRootCommands(t *testing.T) {
	firstBoardDir := filepath.Join(t.TempDir(), "first-board")
	first := newRootCmd()
	firstBuf := new(bytes.Buffer)
	first.SetOut(firstBuf)
	first.SetErr(firstBuf)
	first.SetArgs([]string{
		"board", "create",
		"--dir", firstBoardDir,
		"--title", "Invalid parent",
		"--parent", "../manifest",
	})

	err := first.Execute()
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid local board issue ID")

	secondBoardDir := filepath.Join(t.TempDir(), "second-board")
	second := newRootCmd()
	secondBuf := new(bytes.Buffer)
	second.SetOut(secondBuf)
	second.SetErr(secondBuf)
	second.SetArgs([]string{
		"board", "create",
		"--dir", secondBoardDir,
		"--title", "Fresh create",
	})

	require.NoError(t, second.Execute())
	assert.Equal(t, "CB-1", strings.TrimSpace(secondBuf.String()))
}

func TestBoardCommandRejectsUnsafeIssueIDsBeforeOpeningBoard(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "show",
			args: []string{"board", "show", "../manifest"},
		},
		{
			name: "move",
			args: []string{"board", "move", "../manifest", "done"},
		},
		{
			name: "comment",
			args: []string{"board", "comment", "../manifest", "--body", "unsafe"},
		},
		{
			name: "assign",
			args: []string{"board", "assign", "../manifest", "worker-a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boardDir := filepath.Join(t.TempDir(), "board")
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(append(tt.args, "--dir", boardDir))

			err := cmd.Execute()
			require.Error(t, err)
			assert.ErrorContains(t, err, "invalid local board issue ID")
			assert.ErrorContains(t, err, "../manifest")
			_, statErr := os.Stat(boardDir)
			assert.True(t, errors.Is(statErr, os.ErrNotExist), "invalid issue IDs should not initialize board storage")
		})
	}
}

func TestRunBoardCreateRejectsUnsafeRelatedIssueIDsBeforeOpeningBoard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		parentID  string
		blockedBy []string
	}{
		{
			name:     "parent",
			parentID: "../manifest",
		},
		{
			name:      "blocked-by",
			blockedBy: []string{"../manifest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boardDir := filepath.Join(t.TempDir(), "board")
			cmd := newBoardCreateTestCommand(boardDir, tt.parentID, tt.blockedBy)

			err := runBoardCreate(cmd, nil)
			require.Error(t, err)
			assert.ErrorContains(t, err, "invalid local board issue ID")
			assert.ErrorContains(t, err, "../manifest")
			_, statErr := os.Stat(boardDir)
			assert.True(t, errors.Is(statErr, os.ErrNotExist), "invalid issue IDs should not initialize board storage")
		})
	}
}

func newBoardCreateTestCommand(boardDir string, parentID string, blockedBy []string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "path to WORKFLOW.md file")
	cmd.Flags().String("dir", boardDir, "override internal board directory")
	cmd.Flags().String("prefix", "", "override local issue prefix")
	cmd.Flags().String("title", "unsafe", "issue title")
	cmd.Flags().String("description", "", "issue description")
	cmd.Flags().String("parent", parentID, "parent issue ID")
	cmd.Flags().String("assignee", "", "assign the issue to a worker or team")
	cmd.Flags().StringSlice("labels", nil, "issue labels")
	cmd.Flags().StringSlice("blocked-by", blockedBy, "board issue IDs that block this issue")
	return cmd
}

func TestBoardDispatchUntilEmptyDrainsRunnableIssues(t *testing.T) {
	boardDir := filepath.Join(t.TempDir(), "board")
	cfgPath := writeBoardWorkflowConfig(t, boardDir)

	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    boardDir,
		IssuePrefix: "CB",
		Actor:       "test-bot",
	})

	ctx := context.Background()
	_, err := localTracker.InitBoard(ctx)
	require.NoError(t, err)

	parent, err := localTracker.CreateIssueWithOptions(ctx, tracker.LocalIssueCreateOptions{
		Title:    "Parent issue",
		Assignee: "team-alpha",
	})
	require.NoError(t, err)

	child, err := localTracker.CreateIssueWithOptions(ctx, tracker.LocalIssueCreateOptions{
		Title:     "Child issue",
		ParentID:  parent.ID,
		Assignee:  "team-alpha",
		BlockedBy: []string{parent.ID},
	})
	require.NoError(t, err)

	var calls []teamRunOptions
	err = dispatchBoardIssues(
		ctx,
		new(bytes.Buffer),
		localTracker,
		boardDispatchOptions{
			ConfigPath: cfgPath,
			UntilEmpty: true,
		},
		func(opts teamRunOptions) error {
			calls = append(calls, opts)
			return localTracker.UpdateIssueState(ctx, opts.IssueID, types.Released)
		},
	)
	require.NoError(t, err)

	require.Len(t, calls, 2)
	assert.Equal(t, []string{parent.ID, child.ID}, []string{calls[0].IssueID, calls[1].IssueID})
	assert.Equal(t, []string{"team-alpha", "team-alpha"}, []string{calls[0].TeamName, calls[1].TeamName})

	parent, err = localTracker.GetIssue(ctx, parent.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateDone, parent.State)

	child, err = localTracker.GetIssue(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateDone, child.State)
}

func TestBoardDispatchUntilEmptyCommandReportsDrainSummary(t *testing.T) {
	boardDir := filepath.Join(t.TempDir(), "board")
	cfgPath := writeBoardWorkflowConfig(t, boardDir)

	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    boardDir,
		IssuePrefix: "CB",
		Actor:       "test-bot",
	})
	ctx := context.Background()
	_, err := localTracker.InitBoard(ctx)
	require.NoError(t, err)

	issueOne, err := localTracker.CreateIssueWithOptions(ctx, tracker.LocalIssueCreateOptions{
		Title:    "First issue",
		Assignee: "team-alpha",
	})
	require.NoError(t, err)

	issueTwo, err := localTracker.CreateIssueWithOptions(ctx, tracker.LocalIssueCreateOptions{
		Title:    "Second issue",
		Assignee: "team-beta",
	})
	require.NoError(t, err)

	originalRunTeam := runBoardDispatchTeam
	runBoardDispatchTeam = func(opts teamRunOptions) error {
		return localTracker.UpdateIssueState(ctx, opts.IssueID, types.Released)
	}
	defer func() {
		runBoardDispatchTeam = originalRunTeam
	}()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"board", "dispatch",
		"--config", cfgPath,
		"--dir", boardDir,
		"--until-empty",
	})

	require.NoError(t, cmd.Execute())

	output := buf.String()
	assert.Contains(t, output, fmt.Sprintf("dispatched %s to team-alpha", issueOne.ID))
	assert.Contains(t, output, fmt.Sprintf("dispatched %s to team-beta", issueTwo.ID))
	assert.Contains(t, output, "drained board after 2 dispatches")
}

func TestBoardDispatchUntilEmptyCommandSucceedsWhenBoardIsAlreadyDrained(t *testing.T) {
	boardDir := filepath.Join(t.TempDir(), "board")
	cfgPath := writeBoardWorkflowConfig(t, boardDir)

	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    boardDir,
		IssuePrefix: "CB",
		Actor:       "test-bot",
	})
	_, err := localTracker.InitBoard(context.Background())
	require.NoError(t, err)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"board", "dispatch",
		"--config", cfgPath,
		"--dir", boardDir,
		"--until-empty",
	})

	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "board already drained")
}

func writeBoardWorkflowConfig(t *testing.T, boardDir string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "WORKFLOW.md")
	content := fmt.Sprintf(`---
tracker:
  type: internal
  board_dir: %q
---
Internal board test workflow.
`, boardDir)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}
