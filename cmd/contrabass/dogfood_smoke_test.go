package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/agent"
	"github.com/junhoyeo/contrabass/internal/tracker"
	"github.com/junhoyeo/contrabass/internal/types"
	"github.com/junhoyeo/contrabass/internal/workspace"
)

func TestLocalDogfoodSmokeBoardWorkspaceAndMockAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repoDir := initDogfoodGitRepo(t)
	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    filepath.Join(repoDir, ".contrabass", "board"),
		IssuePrefix: "CB",
		Actor:       "operator",
	})
	workspaceManager := workspace.NewManager(repoDir)
	mockAgent := &agent.MockRunner{
		Events: []types.AgentEvent{
			{Type: "session/started"},
			{Type: "turn/completed"},
		},
	}

	manifest, err := localTracker.InitBoard(ctx)
	require.NoError(t, err)
	assert.Equal(t, "CB", manifest.IssuePrefix)

	created, err := localTracker.CreateIssue(ctx, "Dogfood local workflow", "Exercise board, workspace, and fake agent.", []string{"dogfood"})
	require.NoError(t, err)

	fetched, err := localTracker.FetchIssues(ctx)
	require.NoError(t, err)
	require.Len(t, fetched, 1)
	assert.Equal(t, created.ID, fetched[0].ID)
	assert.Equal(t, types.Unclaimed, fetched[0].State)

	require.NoError(t, localTracker.ClaimIssue(ctx, created.ID))
	claimed, err := localTracker.GetIssue(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateInProgress, claimed.State)
	assert.Equal(t, "operator", claimed.ClaimedBy)

	require.NoError(t, localTracker.ReleaseIssue(ctx, created.ID))
	released, err := localTracker.GetIssue(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateInProgress, released.State)
	assert.Empty(t, released.ClaimedBy)

	require.NoError(t, localTracker.ClaimIssue(ctx, created.ID))
	reclaimed, err := localTracker.GetIssue(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateInProgress, reclaimed.State)
	assert.Equal(t, "operator", reclaimed.ClaimedBy)

	workspacePath, err := workspaceManager.Create(ctx, fetched[0])
	require.NoError(t, err)
	assert.DirExists(t, workspacePath)
	assert.Equal(t, filepath.Join(repoDir, "workspaces", created.ID), workspacePath)
	assert.Equal(t, created.ID, dogfoodGit(t, workspacePath, "branch", "--show-current"))

	process, err := mockAgent.Start(ctx, fetched[0], workspacePath, "Fix issue: "+created.Title)
	require.NoError(t, err)

	var eventTypes []string
	for event := range process.Events {
		eventTypes = append(eventTypes, event.Type)
	}
	require.NoError(t, <-process.Done)
	assert.Equal(t, []string{"session/started", "turn/completed"}, eventTypes)

	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "agent-output.txt"), []byte("local smoke output\n"), 0o644))
	dogfoodGit(t, workspacePath, "add", "agent-output.txt")
	dogfoodGit(t, workspacePath, "commit", "-m", "agent progress")

	require.NoError(t, localTracker.UpdateIssueState(ctx, created.ID, types.Released))
	require.NoError(t, workspaceManager.Cleanup(ctx, created.ID))

	done, err := localTracker.GetIssue(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateDone, done.State)
	assert.Empty(t, done.ClaimedBy)

	openIssues, err := localTracker.FetchIssues(ctx)
	require.NoError(t, err)
	assert.Empty(t, openIssues)
	assert.NoDirExists(t, workspacePath)
	assert.NotContains(t, dogfoodGit(t, repoDir, "worktree", "list", "--porcelain"), workspacePath)
	assert.Empty(t, dogfoodGit(t, repoDir, "status", "--short"))
}

func initDogfoodGitRepo(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	dogfoodGit(t, repoDir, "init")
	dogfoodGit(t, repoDir, "config", "user.email", "operator@example.test")
	dogfoodGit(t, repoDir, "config", "user.name", "Local Operator")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte(".contrabass/board/\nworkspaces/\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Dogfood Repo\n"), 0o644))
	dogfoodGit(t, repoDir, "add", ".gitignore", "README.md")
	dogfoodGit(t, repoDir, "commit", "-m", "init")
	return repoDir
}

func dogfoodGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(output))
	return strings.TrimSpace(string(output))
}
