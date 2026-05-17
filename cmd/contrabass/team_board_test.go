package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/tracker"
	"github.com/junhoyeo/contrabass/internal/types"
)

func TestDefaultTeamNameForIssue(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "issue-cb-12", defaultTeamNameForIssue("CB-12"))
	assert.Equal(t, "issue-issue-9-alpha", defaultTeamNameForIssue("Issue 9 / alpha"))
	assert.Equal(t, "issue-issue", defaultTeamNameForIssue("   "))
}

func TestResolveTeamNameForIssue(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "team-alpha", resolveTeamNameForIssue(tracker.LocalBoardIssue{
		ID:       "CB-1",
		Assignee: "Team Alpha",
	}, ""))
	assert.Equal(t, "ops", resolveTeamNameForIssue(tracker.LocalBoardIssue{
		ID: "CB-2",
	}, "Ops"))
	assert.Equal(t, "issue-cb-3", resolveTeamNameForIssue(tracker.LocalBoardIssue{
		ID: "CB-3",
	}, ""))
}

func TestBuildTeamTasksFromBoardIssue(t *testing.T) {
	t.Parallel()

	issue := tracker.LocalBoardIssue{
		ID:          "CB-12",
		Title:       "Ship autonomous board sync",
		Description: "Add automatic board status updates from team lifecycle events.",
		State:       tracker.LocalBoardStateTodo,
		Assignee:    "team-alpha",
		Labels:      []string{"tracker", "team"},
		URL:         "local://CB-12",
		BlockedBy:   []string{"CB-9"},
	}

	tasks := buildTeamTasksFromBoardIssue(issue)
	require.Len(t, tasks, 3)

	assert.Equal(t, "001-cb-12-plan", tasks[0].ID)
	assert.Equal(t, "002-cb-12-prd", tasks[1].ID)
	assert.Equal(t, "003-cb-12-exec", tasks[2].ID)
	assert.Empty(t, tasks[0].DependsOn)
	assert.Equal(t, []string{"001-cb-12-plan"}, tasks[1].DependsOn)
	assert.Equal(t, []string{"002-cb-12-prd"}, tasks[2].DependsOn)
	assert.Contains(t, tasks[2].Description, "Issue ID: CB-12")
	assert.Contains(t, tasks[2].Description, "Add automatic board status updates")
	assert.Contains(t, tasks[2].Description, "Assigned to: team-alpha")
	assert.Contains(t, tasks[2].Description, "Blocked by: CB-9")
}

func TestBuildBoardTeamPlanUsesChildIssues(t *testing.T) {
	t.Parallel()

	parent := tracker.LocalBoardIssue{
		ID:          "CB-12",
		Title:       "Ship autonomous board sync",
		Description: "Coordinate multiple child tickets.",
		State:       tracker.LocalBoardStateTodo,
		Assignee:    "team-alpha",
	}
	children := []tracker.LocalBoardIssue{
		{
			ID:          "CB-13",
			ParentID:    "CB-12",
			Title:       "Build planner",
			Description: "Create the first child implementation slice.",
			State:       tracker.LocalBoardStateTodo,
		},
		{
			ID:          "CB-14",
			ParentID:    "CB-12",
			Title:       "Wire lifecycle sync",
			Description: "Sync child tickets from team events.",
			State:       tracker.LocalBoardStateRetry,
			BlockedBy:   []string{"CB-13"},
		},
		{
			ID:       "CB-15",
			ParentID: "CB-12",
			Title:    "Already shipped",
			State:    tracker.LocalBoardStateDone,
		},
	}

	plan := buildBoardTeamPlan(parent, children)
	require.Len(t, plan.Tasks, 4)

	assert.Equal(t, "001-cb-12-plan", plan.Tasks[0].ID)
	assert.Equal(t, "002-cb-12-prd", plan.Tasks[1].ID)
	assert.Equal(t, "003-cb-13-exec", plan.Tasks[2].ID)
	assert.Equal(t, "004-cb-14-exec", plan.Tasks[3].ID)
	assert.Equal(t, []string{"002-cb-12-prd"}, plan.Tasks[2].DependsOn)
	assert.Equal(t, []string{"002-cb-12-prd", "003-cb-13-exec"}, plan.Tasks[3].DependsOn)
	assert.Contains(t, plan.Tasks[2].Description, "Parent issue:")
	assert.Contains(t, plan.Tasks[2].Description, "Child issue:")
	assert.Equal(t, map[string]string{
		"003-cb-13-exec": "CB-13",
		"004-cb-14-exec": "CB-14",
	}, plan.TaskIssueIDs)
}

func TestAppendUniqueString(t *testing.T) {
	tests := []struct {
		name      string
		values    []string
		candidate string
		want      []string
	}{
		{name: "appends new value", values: []string{"CB-1"}, candidate: "CB-2", want: []string{"CB-1", "CB-2"}},
		{name: "keeps existing value once", values: []string{"CB-1", "CB-2"}, candidate: "CB-2", want: []string{"CB-1", "CB-2"}},
		{name: "allows empty candidate", values: nil, candidate: "", want: []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, appendUniqueString(tt.values, tt.candidate))
		})
	}
}

func TestStringFromMap(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]interface{}
		key    string
		want   string
	}{
		{name: "nil map", values: nil, key: "task_id", want: ""},
		{name: "missing key", values: map[string]interface{}{"other": "value"}, key: "task_id", want: ""},
		{name: "string value", values: map[string]interface{}{"task_id": "task-1"}, key: "task_id", want: "task-1"},
		{name: "numeric value", values: map[string]interface{}{"attempt": 2}, key: "attempt", want: "2"},
		{name: "bool value", values: map[string]interface{}{"ok": true}, key: "ok", want: "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stringFromMap(tt.values, tt.key))
		})
	}
}

func TestBoardIssueSyncerLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    filepath.Join(t.TempDir(), "board"),
		IssuePrefix: "CB",
		Actor:       "team:issue-cb-1",
	})

	_, err := localTracker.InitBoard(ctx)
	require.NoError(t, err)

	issue, err := localTracker.CreateIssue(ctx, "Ship board sync", "Wire team events back into the board", []string{"team"})
	require.NoError(t, err)

	syncer := newBoardIssueSyncer(localTracker, issue.ID, "issue-cb-1", nil)
	require.NoError(t, syncer.Start(ctx))

	syncer.HandleEvent(ctx, types.TeamEvent{
		Type:      "phase_started",
		TeamName:  "issue-cb-1",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"phase": string(types.PhaseExec),
		},
	})
	syncer.HandleEvent(ctx, types.TeamEvent{
		Type:      "task_claimed",
		TeamName:  "issue-cb-1",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"worker_id": "worker-1",
			"task_id":   "003-cb-1-exec",
			"task":      "Implement CB-1",
		},
	})
	syncer.HandleEvent(ctx, types.TeamEvent{
		Type:      "pipeline_completed",
		TeamName:  "issue-cb-1",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"phase": string(types.PhaseComplete),
		},
	})

	updated, err := localTracker.GetIssue(ctx, issue.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateDone, updated.State)
	assert.Empty(t, updated.ClaimedBy)
	assert.Equal(t, "issue-cb-1", updated.TrackerMeta["team_name"])
	assert.Equal(t, "complete", updated.TrackerMeta["team_status"])
	assert.Equal(t, string(types.PhaseExec), updated.TrackerMeta["team_phase"])
	assert.Equal(t, "worker-1", updated.TrackerMeta["last_worker_id"])

	comments, err := localTracker.ListComments(ctx, issue.ID)
	require.NoError(t, err)
	require.NotEmpty(t, comments)

	var bodies []string
	for _, comment := range comments {
		bodies = append(bodies, comment.Body)
	}
	assert.Contains(t, strings.Join(bodies, "\n"), "team issue-cb-1 started execution")
	assert.Contains(t, strings.Join(bodies, "\n"), "entered phase team-exec")
	assert.Contains(t, strings.Join(bodies, "\n"), "completed with phase complete")
}

func TestBoardIssueSyncerFinalizeErrorMarksRetry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    filepath.Join(t.TempDir(), "board"),
		IssuePrefix: "CB",
		Actor:       "team:issue-cb-2",
	})

	_, err := localTracker.InitBoard(ctx)
	require.NoError(t, err)

	issue, err := localTracker.CreateIssue(ctx, "Retry me", "This run should be marked for retry", nil)
	require.NoError(t, err)

	syncer := newBoardIssueSyncer(localTracker, issue.ID, "issue-cb-2", nil)
	require.NoError(t, syncer.Start(ctx))

	syncer.Finalize(ctx, errors.New("boom"))

	updated, err := localTracker.GetIssue(ctx, issue.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateRetry, updated.State)
	assert.Equal(t, "retry", updated.TrackerMeta["team_status"])

	comments, err := localTracker.ListComments(ctx, issue.ID)
	require.NoError(t, err)
	require.NotEmpty(t, comments)
	assert.Contains(t, comments[len(comments)-1].Body, "ended with error: boom")
}

func TestBoardIssueSyncerMarkClaimedChildIssuesForRetry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    filepath.Join(t.TempDir(), "board"),
		IssuePrefix: "CB",
		Actor:       "team:issue-cb-10",
	})

	_, err := localTracker.InitBoard(ctx)
	require.NoError(t, err)

	parent, err := localTracker.CreateIssue(ctx, "Parent", "Parent issue", nil)
	require.NoError(t, err)
	childInProgress, err := localTracker.CreateIssueWithOptions(ctx, tracker.LocalIssueCreateOptions{
		Title:    "Claimed child",
		ParentID: parent.ID,
	})
	require.NoError(t, err)
	childTodo, err := localTracker.CreateIssueWithOptions(ctx, tracker.LocalIssueCreateOptions{
		Title:    "Queued child",
		ParentID: parent.ID,
	})
	require.NoError(t, err)

	_, err = localTracker.UpdateIssue(ctx, childInProgress.ID, func(issue *tracker.LocalBoardIssue) error {
		issue.State = tracker.LocalBoardStateInProgress
		issue.ClaimedBy = "worker-1"
		return nil
	})
	require.NoError(t, err)

	syncer := newBoardIssueSyncer(localTracker, parent.ID, "issue-cb-10", map[string]string{
		"003-cb-11-exec": childInProgress.ID,
		"004-cb-12-exec": childTodo.ID,
		"parent":         parent.ID,
		"empty":          "",
	})

	syncer.markClaimedChildIssuesForRetry(ctx, errors.New("boom"))

	updatedChild, err := localTracker.GetIssue(ctx, childInProgress.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateRetry, updatedChild.State)
	assert.Empty(t, updatedChild.ClaimedBy)
	assert.Equal(t, "issue-cb-10", updatedChild.TrackerMeta["team_name"])
	assert.Equal(t, parent.ID, updatedChild.TrackerMeta["parent_issue_id"])
	assert.Equal(t, "retry", updatedChild.TrackerMeta["team_status"])
	assert.Equal(t, "run_error", updatedChild.TrackerMeta["last_team_event"])
	assert.NotEmpty(t, updatedChild.TrackerMeta["last_team_event_at"])

	childComments, err := localTracker.ListComments(ctx, childInProgress.ID)
	require.NoError(t, err)
	require.NotEmpty(t, childComments)
	assert.Contains(t, childComments[len(childComments)-1].Body, "team issue-cb-10 aborted while executing child issue: boom")

	unchangedChild, err := localTracker.GetIssue(ctx, childTodo.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateTodo, unchangedChild.State)

	unchangedComments, err := localTracker.ListComments(ctx, childTodo.ID)
	require.NoError(t, err)
	assert.Empty(t, unchangedComments)
}

func TestBoardIssueSyncerSyncsChildIssues(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    filepath.Join(t.TempDir(), "board"),
		IssuePrefix: "CB",
		Actor:       "team:issue-cb-12",
	})

	_, err := localTracker.InitBoard(ctx)
	require.NoError(t, err)

	parent, err := localTracker.CreateIssue(ctx, "Parent board issue", "Top-level execution ticket", []string{"team"})
	require.NoError(t, err)
	childOne, err := localTracker.CreateIssueWithOptions(ctx, tracker.LocalIssueCreateOptions{
		Title:    "Child one",
		ParentID: parent.ID,
	})
	require.NoError(t, err)
	childTwo, err := localTracker.CreateIssueWithOptions(ctx, tracker.LocalIssueCreateOptions{
		Title:     "Child two",
		ParentID:  parent.ID,
		BlockedBy: []string{childOne.ID},
	})
	require.NoError(t, err)

	plan := buildBoardTeamPlan(parent, []tracker.LocalBoardIssue{childOne, childTwo})
	syncer := newBoardIssueSyncer(localTracker, parent.ID, "issue-cb-12", plan.TaskIssueIDs)
	require.NoError(t, syncer.Start(ctx))

	syncer.HandleEvent(ctx, types.TeamEvent{
		Type:      "task_claimed",
		TeamName:  "issue-cb-12",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"worker_id": "worker-1",
			"task_id":   "003-cb-2-exec",
		},
	})
	syncer.HandleEvent(ctx, types.TeamEvent{
		Type:      "task_completed",
		TeamName:  "issue-cb-12",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"worker_id": "worker-1",
			"task_id":   "003-cb-2-exec",
		},
	})
	syncer.HandleEvent(ctx, types.TeamEvent{
		Type:      "task_failed",
		TeamName:  "issue-cb-12",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"worker_id": "worker-2",
			"task_id":   "004-cb-3-exec",
			"error":     "boom",
		},
	})

	firstChild, err := localTracker.GetIssue(ctx, childOne.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateDone, firstChild.State)
	assert.Equal(t, "complete", firstChild.TrackerMeta["team_status"])

	secondChild, err := localTracker.GetIssue(ctx, childTwo.ID)
	require.NoError(t, err)
	assert.Equal(t, tracker.LocalBoardStateRetry, secondChild.State)
	assert.Equal(t, "retry", secondChild.TrackerMeta["team_status"])
	assert.Equal(t, parent.ID, secondChild.TrackerMeta["parent_issue_id"])

	childComments, err := localTracker.ListComments(ctx, childTwo.ID)
	require.NoError(t, err)
	require.NotEmpty(t, childComments)
	assert.Contains(t, childComments[len(childComments)-1].Body, "failed 004-cb-3-exec")
}
