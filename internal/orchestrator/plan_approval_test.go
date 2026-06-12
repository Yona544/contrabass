package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/approval"
	"github.com/junhoyeo/contrabass/internal/tracker"
	"github.com/junhoyeo/contrabass/internal/types"
	"github.com/junhoyeo/contrabass/internal/workspace"
)

func newApprovalOrchestrator(t *testing.T) (*Orchestrator, *tracker.MockTracker, *approval.Store) {
	t.Helper()

	mt := tracker.NewMockTracker()
	mw := workspace.NewMockManager(t.TempDir())
	orch := NewOrchestrator(mt, mw, nil, nil, nil)
	store := approval.NewStore(t.TempDir())
	orch.SetApprovalStore(store)
	return orch, mt, store
}

func TestApplyApprovalPromptStates(t *testing.T) {
	orch, _, store := newApprovalOrchestrator(t)

	// No state: planning mode.
	prompt, planOnly := orch.applyApprovalPrompt("CB-1", "do the task")
	assert.True(t, planOnly)
	assert.Contains(t, prompt, "PLANNING MODE")
	assert.Contains(t, prompt, "PLAN.md")

	// Approved with plan: plan attached, execute mode.
	require.NoError(t, store.MarkPlanned("CB-1", "1. change x"))
	require.NoError(t, store.Approve("CB-1"))
	prompt, planOnly = orch.applyApprovalPrompt("CB-1", "do the task")
	assert.False(t, planOnly)
	assert.Contains(t, prompt, "Approved plan")
	assert.Contains(t, prompt, "1. change x")

	// Disabled gate: untouched.
	orch.SetApprovalStore(nil)
	prompt, planOnly = orch.applyApprovalPrompt("CB-1", "do the task")
	assert.False(t, planOnly)
	assert.Equal(t, "do the task", prompt)
}

func TestApprovalParkedOnlyForPlannedIssues(t *testing.T) {
	orch, _, store := newApprovalOrchestrator(t)

	assert.False(t, orch.approvalParked("CB-1"), "unplanned issues dispatch (in planning mode)")

	require.NoError(t, store.MarkPlanned("CB-1", "plan"))
	assert.True(t, orch.approvalParked("CB-1"))

	require.NoError(t, store.Approve("CB-1"))
	assert.False(t, orch.approvalParked("CB-1"))
}

func TestFinishPlanRunParksIssueWithPlan(t *testing.T) {
	orch, mt, store := newApprovalOrchestrator(t)

	workspaceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workspaceDir, planFileName), []byte("1. add tests\n2. refactor"), 0o644))

	entry := &runEntry{
		issue:     types.Issue{ID: "CB-7", Identifier: "CB-7", Title: "Do it"},
		workspace: workspaceDir,
		planOnly:  true,
	}
	attempt := types.RunAttempt{Attempt: 1, Phase: types.Succeeded}

	orch.finishPlanRun(context.Background(), entry, attempt)

	status, plan, err := store.Get("CB-7")
	require.NoError(t, err)
	assert.Equal(t, approval.StatusPlanned, status)
	assert.Contains(t, plan, "add tests")

	comments := mt.Comments["CB-7"]
	require.Len(t, comments, 1)
	assert.Contains(t, comments[0], "Plan for approval")
	assert.Contains(t, comments[0], "contrabass approve CB-7")

	assert.Equal(t, types.Unclaimed, mt.States["CB-7"])
}

func TestFinishPlanRunWithoutPlanFileStillParks(t *testing.T) {
	orch, mt, store := newApprovalOrchestrator(t)

	entry := &runEntry{
		issue:     types.Issue{ID: "CB-8", Identifier: "CB-8"},
		workspace: t.TempDir(),
		planOnly:  true,
	}
	orch.finishPlanRun(context.Background(), entry, types.RunAttempt{Attempt: 1, Phase: types.Succeeded})

	status, plan, err := store.Get("CB-8")
	require.NoError(t, err)
	assert.Equal(t, approval.StatusPlanned, status)
	assert.Contains(t, plan, "without writing PLAN.md")
	require.Len(t, mt.Comments["CB-8"], 1)
}

func TestApproveIssueRequiresStore(t *testing.T) {
	orch := NewOrchestrator(tracker.NewMockTracker(), nil, nil, nil, nil)
	require.Error(t, orch.ApproveIssue("CB-1"))

	orch.SetApprovalStore(approval.NewStore(t.TempDir()))
	require.NoError(t, orch.ApproveIssue("CB-1"))
}
