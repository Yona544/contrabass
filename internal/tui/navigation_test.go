package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/junhoyeo/contrabass/internal/orchestrator"
	"github.com/junhoyeo/contrabass/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupModelWithAgents(n int) Model {
	m := NewModel()
	m.width = 100
	m.height = 40
	now := time.Now()

	for i := 0; i < n; i++ {
		id := agentID(i)
		m.agents[id] = AgentRow{
			IssueID:   id,
			Stage:     types.StreamingTurn.String(),
			PID:       1000 + i,
			Age:       "1m",
			Phase:     types.StreamingTurn,
			SessionID: "sess-" + id,
		}
		m.agentStartTime[id] = now
		m.agentEvents[id] = NewEventLog(defaultEventLogSize)
		m.agentEvents[id].Push(EventLogEntry{
			Timestamp: now,
			Type:      "AgentStarted",
			Detail:    "started",
		})
	}
	m.agentSortDirty = true
	m.agentRowsDirty = true
	// Apply a synthetic window size so viewport has dimensions for rendering.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return updated.(Model)
}

func agentID(i int) string {
	return "ISSUE-" + string(rune('A'+i))
}

func TestCursorMovement(t *testing.T) {
	m := setupModelWithAgents(3)

	assert.Equal(t, 0, m.table.Selected())
	assert.Equal(t, ViewOverview, m.viewMode)

	updated, _ := m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	m = updated.(Model)
	assert.Equal(t, 1, m.table.Selected())

	updated, _ = m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	m = updated.(Model)
	assert.Equal(t, 2, m.table.Selected())

	updated, _ = m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	m = updated.(Model)
	assert.Equal(t, 2, m.table.Selected(), "cursor should clamp at bottom")

	updated, _ = m.Update(tea.KeyPressMsg{Text: "k", Code: 'k'})
	m = updated.(Model)
	assert.Equal(t, 1, m.table.Selected())

	updated, _ = m.Update(tea.KeyPressMsg{Text: "k", Code: 'k'})
	m = updated.(Model)
	assert.Equal(t, 0, m.table.Selected())

	updated, _ = m.Update(tea.KeyPressMsg{Text: "k", Code: 'k'})
	m = updated.(Model)
	assert.Equal(t, 0, m.table.Selected(), "cursor should clamp at top")
}

func TestEnterOpensDetailView(t *testing.T) {
	m := setupModelWithAgents(2)
	assert.Equal(t, ViewOverview, m.viewMode)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	assert.Equal(t, ViewDetail, m.viewMode)
}

func TestEscapeClosesDetailView(t *testing.T) {
	m := setupModelWithAgents(2)
	m.viewMode = ViewDetail

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	assert.Equal(t, ViewOverview, m.viewMode)
}

func TestEscapeNoOpInOverview(t *testing.T) {
	m := setupModelWithAgents(2)
	assert.Equal(t, ViewOverview, m.viewMode)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	assert.Equal(t, ViewOverview, m.viewMode)
}

func TestTabCyclesPanels(t *testing.T) {
	m := setupModelWithAgents(2)
	m.teams["team-a"] = TeamRow{TeamName: "team-a", Phase: "team-exec", Workers: 2}
	m.teamWorkers["team-a"] = []TeamWorkerRow{}
	m.teamEvents["team-a"] = NewEventLog(defaultEventLogSize)
	m.teamSortDirty = true
	m.teamRowsDirty = true
	m = m.syncTables()

	assert.Equal(t, PanelAgents, m.focusedPanel)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	assert.Equal(t, PanelTeam, m.focusedPanel)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	assert.Equal(t, PanelAgents, m.focusedPanel, "should wrap around")
}

func TestTabNoOpWithSinglePanel(t *testing.T) {
	m := setupModelWithAgents(2)
	assert.Equal(t, PanelAgents, m.focusedPanel)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	assert.Equal(t, PanelAgents, m.focusedPanel, "single panel should not change")
}

func TestDetailViewShowsAgentInfo(t *testing.T) {
	m := setupModelWithAgents(2)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	view := stripANSI(m.View().Content)
	assert.Contains(t, view, "AGENT")
	assert.Contains(t, view, "EVENT LOG")
}

func TestDetailViewAgentGone(t *testing.T) {
	m := setupModelWithAgents(1)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	assert.Equal(t, ViewDetail, m.viewMode)

	delete(m.agents, agentID(0))
	m.agentKeys = nil
	m.agentSortDirty = true
	m = m.syncTables()

	view := stripANSI(m.View().Content)
	assert.Contains(t, view, "Agent no longer running")
}

func TestCursorMovementOnTeamPanel(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	m.teams["team-a"] = TeamRow{TeamName: "team-a", Phase: "team-exec"}
	m.teams["team-b"] = TeamRow{TeamName: "team-b", Phase: "team-plan"}
	m.teamWorkers["team-a"] = []TeamWorkerRow{}
	m.teamWorkers["team-b"] = []TeamWorkerRow{}
	m.teamEvents["team-a"] = NewEventLog(defaultEventLogSize)
	m.teamEvents["team-b"] = NewEventLog(defaultEventLogSize)
	m.teamSortDirty = true
	m.teamRowsDirty = true
	m.focusedPanel = PanelTeam
	m = m.syncTables()

	assert.Equal(t, 0, m.teamTable.Selected())

	updated, _ = m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	m = updated.(Model)
	assert.Equal(t, 1, m.teamTable.Selected())
}

func TestModelSelectedDetailKeys(t *testing.T) {
	tests := []struct {
		name          string
		focusedPanel  FocusedPanel
		agentKeys     []string
		teamKeys      []string
		agentSelected int
		teamSelected  int
		wantIssueID   string
		wantTeamName  string
	}{
		{
			name:          "agent selection",
			focusedPanel:  PanelAgents,
			agentKeys:     []string{"ISSUE-1", "ISSUE-2"},
			agentSelected: 1,
			wantIssueID:   "ISSUE-2",
		},
		{
			name:         "team selection",
			focusedPanel: PanelTeam,
			teamKeys:     []string{"team-a", "team-b"},
			teamSelected: 0,
			wantTeamName: "team-a",
		},
		{
			name:          "agent helper ignores team panel",
			focusedPanel:  PanelTeam,
			agentKeys:     []string{"ISSUE-1"},
			agentSelected: 0,
			teamKeys:      []string{"team-a"},
			wantTeamName:  "team-a",
		},
		{
			name:          "team helper ignores agent panel",
			focusedPanel:  PanelAgents,
			agentKeys:     []string{"ISSUE-1"},
			agentSelected: 0,
			teamKeys:      []string{"team-a"},
			teamSelected:  0,
			wantIssueID:   "ISSUE-1",
		},
		{
			name:          "negative selections return empty",
			focusedPanel:  PanelAgents,
			agentKeys:     []string{"ISSUE-1"},
			teamKeys:      []string{"team-a"},
			agentSelected: -1,
			teamSelected:  -1,
		},
		{
			name:          "out of range selections return empty",
			focusedPanel:  PanelAgents,
			agentKeys:     []string{"ISSUE-1"},
			teamKeys:      []string{"team-a"},
			agentSelected: 3,
			teamSelected:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			m.focusedPanel = tt.focusedPanel
			m.agentKeys = tt.agentKeys
			m.teamKeys = tt.teamKeys
			m.table = m.table.SetSelected(tt.agentSelected)
			m.teamTable = m.teamTable.SetSelected(tt.teamSelected)

			assert.Equal(t, tt.wantIssueID, m.selectedIssueID())
			assert.Equal(t, tt.wantTeamName, m.selectedTeamName())
		})
	}
}

func TestAgentEventLogPopulated(t *testing.T) {
	m := NewModel()
	now := time.Now()

	event := orchestrator.OrchestratorEvent{
		Type:      orchestrator.EventAgentStarted,
		IssueID:   "ISSUE-1",
		Timestamp: now,
		Data: orchestrator.AgentStarted{
			Attempt:   1,
			PID:       321,
			SessionID: "sess-1",
		},
	}

	updated, _ := m.Update(OrchestratorEventMsg{Event: event})
	model := updated.(Model)

	log, ok := model.agentEvents["ISSUE-1"]
	require.True(t, ok)
	assert.Equal(t, 1, log.Len())

	entries := log.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "AgentStarted", entries[0].Type)
	assert.Contains(t, entries[0].Detail, "PID=321")
}

func TestTeamEventLogPopulated(t *testing.T) {
	m := NewModel()
	now := time.Now()

	updated, _ := m.Update(TeamEventMsg{Event: types.TeamEvent{
		Type:      "team_created",
		TeamName:  "team-alpha",
		Timestamp: now,
		Data: map[string]interface{}{
			"max_workers": 2,
		},
	}})
	model := updated.(Model)

	log, ok := model.teamEvents["team-alpha"]
	require.True(t, ok)
	assert.Equal(t, 1, log.Len())

	entries := log.Entries()
	assert.Equal(t, "team_created", entries[0].Type)
	assert.Contains(t, entries[0].Detail, "workers=2")
}

func TestDetailViewTeam(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)
	m.teams["team-a"] = TeamRow{TeamName: "team-a", Phase: "team-exec", Workers: 2}
	m.teamWorkers["team-a"] = []TeamWorkerRow{
		{WorkerID: "w1", Status: "working", CurrentTask: "task-1"},
	}
	m.teamEvents["team-a"] = NewEventLog(defaultEventLogSize)
	m.teamKeys = []string{"team-a"}
	m.teamSortDirty = true
	m.teamRowsDirty = true
	m.focusedPanel = PanelTeam
	m.viewMode = ViewDetail
	m = m.syncTables()

	view := stripANSI(m.View().Content)
	assert.Contains(t, view, "TEAM")
	assert.Contains(t, view, "WORKERS")
	assert.Contains(t, view, "w1")
}

func TestTableFocusedState(t *testing.T) {
	m := setupModelWithAgents(2)
	assert.True(t, m.table.focused, "agent table should be focused by default")
	assert.False(t, m.teamTable.focused, "team table should not be focused")
}

func TestIssueReleasedCleansUpEventLog(t *testing.T) {
	m := NewModel()
	now := time.Now()

	m.agents["ISSUE-1"] = AgentRow{IssueID: "ISSUE-1"}
	m.agentStartTime["ISSUE-1"] = now
	m.agentEvents["ISSUE-1"] = NewEventLog(defaultEventLogSize)
	m.agentEvents["ISSUE-1"].Push(EventLogEntry{Type: "test"})

	event := orchestrator.OrchestratorEvent{
		Type:      orchestrator.EventIssueReleased,
		IssueID:   "ISSUE-1",
		Timestamp: now,
		Data:      orchestrator.IssueReleased{Attempt: 1},
	}

	updated, _ := m.Update(OrchestratorEventMsg{Event: event})
	model := updated.(Model)
	_, hasLog := model.agentEvents["ISSUE-1"]
	assert.False(t, hasLog, "event log should be cleaned up on release")
}
