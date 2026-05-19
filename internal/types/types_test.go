package types

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestIssueStateString(t *testing.T) {
	tests := []struct {
		state    IssueState
		expected string
	}{
		{Unclaimed, "Unclaimed"},
		{Claimed, "Claimed"},
		{Running, "Running"},
		{RetryQueued, "RetryQueued"},
		{Released, "Released"},
		{IssueState(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestRunPhaseString(t *testing.T) {
	tests := []struct {
		phase    RunPhase
		expected string
	}{
		{PreparingWorkspace, "PreparingWorkspace"},
		{BuildingPrompt, "BuildingPrompt"},
		{LaunchingAgentProcess, "LaunchingAgentProcess"},
		{InitializingSession, "InitializingSession"},
		{StreamingTurn, "StreamingTurn"},
		{Finishing, "Finishing"},
		{Succeeded, "Succeeded"},
		{Failed, "Failed"},
		{TimedOut, "TimedOut"},
		{Stalled, "Stalled"},
		{CanceledByReconciliation, "CanceledByReconciliation"},
		{RunPhase(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.phase.String())
		})
	}
}

func TestRunPhaseLabel(t *testing.T) {
	tests := []struct {
		name     string
		phase    RunPhase
		expected string
	}{
		{"preparing workspace", PreparingWorkspace, "PreparingWorkspace"},
		{"building prompt", BuildingPrompt, "PreparingWorkspace"},
		{"launching agent process", LaunchingAgentProcess, "RunningAgent"},
		{"initializing session", InitializingSession, "RunningAgent"},
		{"streaming turn", StreamingTurn, "AgentRunning"},
		{"finishing", Finishing, "Releasing"},
		{"succeeded", Succeeded, "Done"},
		{"failed", Failed, "Failed"},
		{"timed out", TimedOut, "Failed"},
		{"stalled", Stalled, "Failed"},
		{"canceled by reconciliation", CanceledByReconciliation, "Canceled"},
		{"unknown", RunPhase(999), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.phase.Label())
		})
	}
}

func TestTeamPhaseIsTerminal(t *testing.T) {
	tests := []struct {
		name  string
		phase TeamPhase
		want  bool
	}{
		{name: "plan is active", phase: PhasePlan, want: false},
		{name: "prd is active", phase: PhasePRD, want: false},
		{name: "exec is active", phase: PhaseExec, want: false},
		{name: "verify is active", phase: PhaseVerify, want: false},
		{name: "fix is active", phase: PhaseFix, want: false},
		{name: "complete is terminal", phase: PhaseComplete, want: true},
		{name: "failed is terminal", phase: PhaseFailed, want: true},
		{name: "cancelled is terminal", phase: PhaseCancelled, want: true},
		{name: "unknown is active", phase: TeamPhase("paused"), want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.phase.IsTerminal())
		})
	}
}

func TestTeamPhaseValidTransitions(t *testing.T) {
	tests := []struct {
		name  string
		phase TeamPhase
		want  []TeamPhase
	}{
		{name: "plan", phase: PhasePlan, want: []TeamPhase{PhasePRD, PhaseCancelled}},
		{name: "prd", phase: PhasePRD, want: []TeamPhase{PhaseExec, PhaseCancelled}},
		{name: "exec", phase: PhaseExec, want: []TeamPhase{PhaseVerify, PhaseCancelled}},
		{name: "verify", phase: PhaseVerify, want: []TeamPhase{PhaseFix, PhaseComplete, PhaseFailed, PhaseCancelled}},
		{name: "fix", phase: PhaseFix, want: []TeamPhase{PhaseExec, PhaseVerify, PhaseComplete, PhaseFailed, PhaseCancelled}},
		{name: "complete", phase: PhaseComplete, want: nil},
		{name: "failed", phase: PhaseFailed, want: nil},
		{name: "cancelled", phase: PhaseCancelled, want: nil},
		{name: "unknown", phase: TeamPhase("paused"), want: nil},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.phase.ValidTransitions())
		})
	}
}

func TestIssueStateEnumContiguity(t *testing.T) {
	// Verify that IssueState enum values are contiguous (no gaps)
	expected := []IssueState{Unclaimed, Claimed, Running, RetryQueued, Released}
	for i, state := range expected {
		assert.Equal(t, IssueState(i), state, "IssueState enum values should be contiguous")
	}
}

func TestRunPhaseEnumContiguity(t *testing.T) {
	// Verify that RunPhase enum values are contiguous (no gaps)
	expected := []RunPhase{
		PreparingWorkspace,
		BuildingPrompt,
		LaunchingAgentProcess,
		InitializingSession,
		StreamingTurn,
		Finishing,
		Succeeded,
		Failed,
		TimedOut,
		Stalled,
		CanceledByReconciliation,
	}
	for i, phase := range expected {
		assert.Equal(t, RunPhase(i), phase, "RunPhase enum values should be contiguous")
	}
}

func TestIssueStateCount(t *testing.T) {
	// Verify that IssueState has exactly 5 values
	assert.Equal(t, 5, int(Released)+1, "IssueState should have exactly 5 values")
}

func TestRunPhaseCount(t *testing.T) {
	// Verify that RunPhase has exactly 11 values
	assert.Equal(t, 11, int(CanceledByReconciliation)+1, "RunPhase should have exactly 11 values")
}

func TestIssue_NewFields(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	issue := Issue{
		ID:          "issue-abc",
		Identifier:  "ENG-123",
		Title:       "Fix critical bug",
		Description: "Something is broken",
		State:       Unclaimed,
		Priority:    2,
		Labels:      []string{"bug"},
		URL:         "https://linear.app/team/ENG-123",
		BranchName:  "symphony/eng-123",
		BlockedBy:   []string{"issue-def", "issue-ghi"},
		CreatedAt:   now,
		UpdatedAt:   now.Add(time.Hour),
		TrackerMeta: map[string]interface{}{"linear_state": "Todo"},
	}

	// Verify fields
	assert.Equal(t, "ENG-123", issue.Identifier)
	assert.Equal(t, 2, issue.Priority)
	assert.Equal(t, "symphony/eng-123", issue.BranchName)
	assert.Equal(t, []string{"issue-def", "issue-ghi"}, issue.BlockedBy)
	assert.Equal(t, now, issue.CreatedAt)
	assert.Equal(t, now.Add(time.Hour), issue.UpdatedAt)

	// Verify JSON serialization round-trip
	data, err := json.Marshal(issue)
	require.NoError(t, err)

	var decoded Issue
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, issue.Identifier, decoded.Identifier)
	assert.Equal(t, issue.Priority, decoded.Priority)
	assert.Equal(t, issue.BranchName, decoded.BranchName)
	assert.Equal(t, issue.BlockedBy, decoded.BlockedBy)
	assert.True(t, issue.CreatedAt.Equal(decoded.CreatedAt))
	assert.True(t, issue.UpdatedAt.Equal(decoded.UpdatedAt))
}

func TestRunAttempt_NewFields(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	run := RunAttempt{
		IssueID:         "issue-abc",
		IssueIdentifier: "ENG-123",
		Phase:           PreparingWorkspace,
		Attempt:         1,
		PID:             1234,
		StartTime:       now,
		WorkspacePath:   "/tmp/workspaces/issue-abc",
	}

	// Verify fields
	assert.Equal(t, "ENG-123", run.IssueIdentifier)
	assert.Equal(t, "/tmp/workspaces/issue-abc", run.WorkspacePath)

	// Verify JSON serialization round-trip
	data, err := json.Marshal(run)
	require.NoError(t, err)

	var decoded RunAttempt
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, run.IssueIdentifier, decoded.IssueIdentifier)
	assert.Equal(t, run.WorkspacePath, decoded.WorkspacePath)
	assert.Equal(t, run.IssueID, decoded.IssueID)
	assert.Equal(t, run.Attempt, decoded.Attempt)
}
