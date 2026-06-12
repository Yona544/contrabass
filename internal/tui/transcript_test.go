package tui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/junhoyeo/contrabass/internal/orchestrator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func transcriptEvent(issueID, text string) OrchestratorEventMsg {
	return OrchestratorEventMsg{Event: orchestrator.OrchestratorEvent{
		Type:      orchestrator.EventAgentTranscript,
		IssueID:   issueID,
		Timestamp: time.Now(),
		Data:      orchestrator.AgentTranscript{Text: text},
	}}
}

func TestTranscriptFragmentsAccumulate(t *testing.T) {
	m := setupModelWithAgents(1)
	id := agentID(0)

	updated, _ := m.Update(transcriptEvent(id, "hello "))
	m = updated.(Model)
	updated, _ = m.Update(transcriptEvent(id, "world"))
	m = updated.(Model)

	buf, ok := m.agentTranscripts[id]
	require.True(t, ok, "transcript buffer should be created on first fragment")
	assert.Equal(t, 2, buf.Len())
	assert.Equal(t, []string{"hello ", "world"}, buf.Fragments())
	assert.Equal(t, 0, m.unknownEvents, "transcript events should be handled, not unknown")
}

func TestTranscriptBufferTrimsAtCap(t *testing.T) {
	m := setupModelWithAgents(1)
	id := agentID(0)

	total := defaultTranscriptSize + 25
	for i := 0; i < total; i++ {
		updated, _ := m.Update(transcriptEvent(id, fmt.Sprintf("frag-%03d ", i)))
		m = updated.(Model)
	}

	buf, ok := m.agentTranscripts[id]
	require.True(t, ok)
	assert.Equal(t, defaultTranscriptSize, buf.Len(), "buffer should trim to its cap")

	fragments := buf.Fragments()
	require.Len(t, fragments, defaultTranscriptSize)
	assert.Equal(t, fmt.Sprintf("frag-%03d ", total-defaultTranscriptSize), fragments[0],
		"oldest fragments should be dropped")
	assert.Equal(t, fmt.Sprintf("frag-%03d ", total-1), fragments[len(fragments)-1],
		"newest fragment should be retained")
}

func TestTranscriptToggleShowsSelectedIssueOnly(t *testing.T) {
	m := setupModelWithAgents(2) // ISSUE-A, ISSUE-B

	updated, _ := m.Update(transcriptEvent(agentID(0), "alpha-output"))
	m = updated.(Model)
	updated, _ = m.Update(transcriptEvent(agentID(1), "bravo-output"))
	m = updated.(Model)

	// Move cursor to the second agent and open the transcript pane.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "t", Code: 't'})
	m = updated.(Model)
	assert.Equal(t, ViewTranscript, m.viewMode)

	view := stripANSI(m.View().Content)
	assert.Contains(t, view, "TRANSCRIPT", "pane title should be rendered")
	assert.Contains(t, view, agentID(1), "pane title should contain the selected issue id")
	assert.Contains(t, view, "bravo-output", "selected issue's fragments should be shown")
	assert.NotContains(t, view, "alpha-output", "other issues' fragments must not be shown")

	// `t` toggles the pane closed again.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "t", Code: 't'})
	m = updated.(Model)
	assert.Equal(t, ViewOverview, m.viewMode)
}

func TestTranscriptEscCloses(t *testing.T) {
	m := setupModelWithAgents(1)

	updated, _ := m.Update(tea.KeyPressMsg{Text: "t", Code: 't'})
	m = updated.(Model)
	require.Equal(t, ViewTranscript, m.viewMode)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	assert.Equal(t, ViewOverview, m.viewMode)
}

func TestTranscriptPlaceholderWhenEmpty(t *testing.T) {
	m := setupModelWithAgents(1)

	updated, _ := m.Update(tea.KeyPressMsg{Text: "t", Code: 't'})
	m = updated.(Model)
	require.Equal(t, ViewTranscript, m.viewMode)

	view := stripANSI(m.View().Content)
	assert.Contains(t, view, agentID(0))
	assert.Contains(t, view, transcriptPlaceholder)
}

func TestTranscriptToggleNoAgentsNoOp(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Text: "t", Code: 't'})
	m = updated.(Model)
	assert.Equal(t, ViewOverview, m.viewMode, "transcript should not open without a selected issue")
}

func TestTranscriptClearedOnAgentFinishedAndReleased(t *testing.T) {
	tests := []struct {
		name      string
		eventType orchestrator.EventType
		data      orchestrator.EventPayload
	}{
		{
			name:      "agent finished",
			eventType: orchestrator.EventAgentFinished,
			data:      orchestrator.AgentFinished{Attempt: 1},
		},
		{
			name:      "issue released",
			eventType: orchestrator.EventIssueReleased,
			data:      orchestrator.IssueReleased{Attempt: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := setupModelWithAgents(1)
			id := agentID(0)
			m.agentWorkspaces[id] = `C:\work\issue-a`

			updated, _ := m.Update(transcriptEvent(id, "some output"))
			m = updated.(Model)
			require.Contains(t, m.agentTranscripts, id)

			updated, _ = m.Update(OrchestratorEventMsg{Event: orchestrator.OrchestratorEvent{
				Type:      tt.eventType,
				IssueID:   id,
				Timestamp: time.Now(),
				Data:      tt.data,
			}})
			m = updated.(Model)

			assert.NotContains(t, m.agentTranscripts, id, "transcript buffer should be cleaned up")
			assert.NotContains(t, m.agentWorkspaces, id, "workspace path should be cleaned up")
		})
	}
}

func TestTranscriptBufferRing(t *testing.T) {
	buf := NewTranscriptBuffer(3)
	assert.Equal(t, 0, buf.Len())
	assert.Nil(t, buf.Fragments())

	buf.Push("a")
	buf.Push("b")
	buf.Push("c")
	buf.Push("d")
	assert.Equal(t, 3, buf.Len())
	assert.Equal(t, []string{"b", "c", "d"}, buf.Fragments())
}
