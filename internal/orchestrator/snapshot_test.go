package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/types"
)

func TestPhaseLabel_Coverage(t *testing.T) {
	phases := []types.RunPhase{
		types.PreparingWorkspace,
		types.BuildingPrompt,
		types.LaunchingAgentProcess,
		types.InitializingSession,
		types.StreamingTurn,
		types.Finishing,
		types.Succeeded,
		types.Failed,
		types.TimedOut,
		types.Stalled,
		types.CanceledByReconciliation,
	}

	for _, phase := range phases {
		t.Run(phase.String(), func(t *testing.T) {
			assert.NotEmpty(t, phase.Label())
		})
	}
	assert.Empty(t, types.RunPhase(999).Label())
}

func TestSnapshot_ReturnsCorrectStats(t *testing.T) {
	o := &Orchestrator{
		running:    make(map[string]*runEntry),
		backoff:    []types.BackoffEntry{},
		issueCache: make(map[string]types.Issue),
		stats: Stats{
			Running:        5,
			MaxAgents:      10,
			TotalTokensIn:  1000,
			TotalTokensOut: 2000,
			StartTime:      time.Now().Add(-1 * time.Hour),
			PollCount:      42,
		},
	}

	snapshot := o.Snapshot()

	assert.Equal(t, 5, snapshot.Stats.Running)
	assert.Equal(t, 10, snapshot.Stats.MaxAgents)
	assert.Equal(t, int64(1000), snapshot.Stats.TotalTokensIn)
	assert.Equal(t, int64(2000), snapshot.Stats.TotalTokensOut)
	assert.Equal(t, 42, snapshot.Stats.PollCount)
	assert.NotZero(t, snapshot.GeneratedAt)
}

func TestSnapshot_IncludesRunningEntries(t *testing.T) {
	now := time.Now()
	issue := types.Issue{
		ID:    "issue-1",
		Title: "Test Issue",
	}
	attempt := types.RunAttempt{
		IssueID:   "issue-1",
		Attempt:   1,
		PID:       12345,
		SessionID: "session-abc",
		StartTime: now,
		Phase:     types.StreamingTurn,
		TokensIn:  100,
		TokensOut: 200,
	}
	entry := &runEntry{
		issue:     issue,
		attempt:   attempt,
		workspace: "/tmp/workspace",
	}

	o := &Orchestrator{
		running:    map[string]*runEntry{"issue-1": entry},
		backoff:    []types.BackoffEntry{},
		issueCache: make(map[string]types.Issue),
		stats:      Stats{},
	}

	snapshot := o.Snapshot()

	require.Len(t, snapshot.Running, 1)
	assert.Equal(t, "issue-1", snapshot.Running[0].IssueID)
	assert.Equal(t, 1, snapshot.Running[0].Attempt)
	assert.Equal(t, 12345, snapshot.Running[0].PID)
	assert.Equal(t, "session-abc", snapshot.Running[0].SessionID)
	assert.Equal(t, "/tmp/workspace", snapshot.Running[0].Workspace)
	assert.Equal(t, now, snapshot.Running[0].StartedAt)
	assert.Equal(t, types.StreamingTurn, snapshot.Running[0].Phase)
	assert.Equal(t, int64(100), snapshot.Running[0].TokensIn)
	assert.Equal(t, int64(200), snapshot.Running[0].TokensOut)
}

func TestSnapshot_LastActivityIgnoresHeartbeats(t *testing.T) {
	orch := NewOrchestrator(nil, nil, nil, nil, nil)
	issueID := "ISS-ACTIVITY"
	toolCallAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	heartbeatAt := toolCallAt

	orch.mu.Lock()
	orch.running[issueID] = &runEntry{
		issue:     types.Issue{ID: issueID},
		attempt:   types.RunAttempt{IssueID: issueID, Attempt: 1, Phase: types.StreamingTurn},
		workspace: t.TempDir(),
	}
	orch.stats.Running = 1
	orch.mu.Unlock()

	orch.handleAgentEvent(issueID, types.AgentEvent{Type: "tool_call", Timestamp: toolCallAt})
	for i := 1; i <= 5; i++ {
		heartbeatAt = toolCallAt.Add(time.Duration(i) * time.Second)
		orch.handleAgentEvent(issueID, types.AgentEvent{Type: "team/stalled", Timestamp: heartbeatAt})
	}

	snapshot := orch.Snapshot()

	require.Len(t, snapshot.Running, 1)
	assert.Equal(t, toolCallAt.Format(time.RFC3339), snapshot.Running[0].LastActivityAt)
	assert.Equal(t, "tool_call", snapshot.Running[0].LastActivityKind)
	assert.Equal(t, heartbeatAt.Format(time.RFC3339), snapshot.Running[0].LastHeartbeatAt)
}

func TestDiffStat_ParsesShortstat(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		wantAdded   int
		wantRemoved int
		wantFiles   int
	}{
		{name: "empty", output: "", wantAdded: 0, wantRemoved: 0, wantFiles: 0},
		{name: "insertions and deletions", output: " 2 files changed, 47 insertions(+), 3 deletions(-)\n", wantAdded: 47, wantRemoved: 3, wantFiles: 2},
		{name: "insertions only", output: " 1 file changed, 5 insertions(+)\n", wantAdded: 5, wantRemoved: 0, wantFiles: 1},
		{name: "deletions only", output: " 3 files changed, 8 deletions(-)\n", wantAdded: 0, wantRemoved: 8, wantFiles: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added, removed, files := parseDiffShortstat(tt.output)
			assert.Equal(t, tt.wantAdded, added)
			assert.Equal(t, tt.wantRemoved, removed)
			assert.Equal(t, tt.wantFiles, files)
		})
	}
}

func TestDiffStat_TimeoutReturnsTimeoutStatus(t *testing.T) {
	if os.Getenv("CONTRABASS_DIFFSTAT_SLEEP") == "1" {
		time.Sleep(2 * time.Second)
		return
	}

	original := diffStatCommand
	diffStatCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestDiffStat_TimeoutReturnsTimeoutStatus")
		cmd.Env = append(os.Environ(), "CONTRABASS_DIFFSTAT_SLEEP=1")
		return cmd
	}
	t.Cleanup(func() {
		diffStatCommand = original
	})

	added, removed, files, status := diffStat(context.Background(), t.TempDir())

	assert.Zero(t, added)
	assert.Zero(t, removed)
	assert.Zero(t, files)
	assert.Equal(t, "timeout", status)
}

func TestReadIterationProgress(t *testing.T) {
	tests := []struct {
		name          string
		content       *string
		wantIteration int
		wantMax       int
	}{
		{name: "missing"},
		{name: "well formed", content: stringPtr(`{"iteration":3,"max_iterations":50}`), wantIteration: 3, wantMax: 50},
		{name: "corrupt", content: stringPtr(`{"iteration":`), wantIteration: 0, wantMax: 0},
		{name: "partial", content: stringPtr(`{"iteration":7}`), wantIteration: 7, wantMax: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			if tt.content != nil {
				stateDir := filepath.Join(workspace, ".omx", "state")
				require.NoError(t, os.MkdirAll(stateDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(stateDir, "run-state.json"), []byte(*tt.content), 0o644))
			}

			iteration, max := readIterationProgress(workspace)

			assert.Equal(t, tt.wantIteration, iteration)
			assert.Equal(t, tt.wantMax, max)
		})
	}
}

func stringPtr(value string) *string {
	return &value
}

func TestSnapshot_IncludesBackoffEntries(t *testing.T) {
	retryTime := time.Now().Add(5 * time.Minute)
	backoffEntry := types.BackoffEntry{
		IssueID: "issue-2",
		Attempt: 2,
		RetryAt: retryTime,
		Error:   "timeout",
	}

	o := &Orchestrator{
		running:    make(map[string]*runEntry),
		backoff:    []types.BackoffEntry{backoffEntry},
		issueCache: make(map[string]types.Issue),
		stats:      Stats{},
	}

	snapshot := o.Snapshot()

	require.Len(t, snapshot.Backoff, 1)
	assert.Equal(t, "issue-2", snapshot.Backoff[0].IssueID)
	assert.Equal(t, 2, snapshot.Backoff[0].Attempt)
	assert.Equal(t, retryTime, snapshot.Backoff[0].RetryAt)
	assert.Equal(t, "timeout", snapshot.Backoff[0].Error)
}

func TestSnapshot_IsIsolatedFromState(t *testing.T) {
	issue := types.Issue{
		ID:    "issue-1",
		Title: "Original Title",
	}

	o := &Orchestrator{
		running:    make(map[string]*runEntry),
		backoff:    []types.BackoffEntry{},
		issueCache: map[string]types.Issue{"issue-1": issue},
		stats:      Stats{Running: 1},
	}

	snapshot := o.Snapshot()

	assert.Equal(t, "Original Title", snapshot.Issues["issue-1"].Title)
	assert.Equal(t, 1, snapshot.Stats.Running)

	o.mu.Lock()
	o.issueCache["issue-1"] = types.Issue{
		ID:    "issue-1",
		Title: "Modified Title",
	}
	o.stats.Running = 5
	o.mu.Unlock()

	assert.Equal(t, "Original Title", snapshot.Issues["issue-1"].Title)
	assert.Equal(t, 1, snapshot.Stats.Running)
}
