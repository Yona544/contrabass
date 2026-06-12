package tui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/junhoyeo/contrabass/internal/orchestrator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubEditorEnv stubs the launcher and PATH lookup, restoring them on cleanup.
// It returns pointers to the captured launch arguments.
func stubEditorEnv(t *testing.T, codeOnPath bool) (launched *bool, gotName *string, gotArgs *[]string) {
	t.Helper()
	launched = new(bool)
	gotName = new(string)
	gotArgs = new([]string)

	origLaunch := launchEditor
	origLookPath := editorLookPath
	t.Cleanup(func() {
		launchEditor = origLaunch
		editorLookPath = origLookPath
	})

	launchEditor = func(name string, args []string) error {
		*launched = true
		*gotName = name
		*gotArgs = args
		return nil
	}
	editorLookPath = func(file string) (string, error) {
		if codeOnPath && file == "code" {
			return `C:\bin\code.cmd`, nil
		}
		return "", errors.New("not found")
	}
	return launched, gotName, gotArgs
}

func TestOpenEditorResolutionOrder(t *testing.T) {
	const workspacePath = `C:\work\issue-a`

	tests := []struct {
		name         string
		visual       string
		editor       string
		codeOnPath   bool
		wantLaunched bool
		wantName     string
		wantArgs     []string
	}{
		{
			name:         "VISUAL wins over EDITOR",
			visual:       "myvisual --reuse-window",
			editor:       "myeditor",
			codeOnPath:   true,
			wantLaunched: true,
			wantName:     "myvisual",
			wantArgs:     []string{"--reuse-window", workspacePath},
		},
		{
			name:         "EDITOR used when VISUAL unset",
			visual:       "",
			editor:       "myeditor",
			codeOnPath:   true,
			wantLaunched: true,
			wantName:     "myeditor",
			wantArgs:     []string{workspacePath},
		},
		{
			name:         "code fallback when neither set",
			visual:       "",
			editor:       "",
			codeOnPath:   true,
			wantLaunched: true,
			wantName:     "code",
			wantArgs:     []string{workspacePath},
		},
		{
			name:         "no editor available",
			visual:       "",
			editor:       "",
			codeOnPath:   false,
			wantLaunched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("VISUAL", tt.visual)
			t.Setenv("EDITOR", tt.editor)
			launched, gotName, gotArgs := stubEditorEnv(t, tt.codeOnPath)

			m := setupModelWithAgents(1)
			m.agentWorkspaces[agentID(0)] = workspacePath

			updated, _ := m.Update(tea.KeyPressMsg{Text: "o", Code: 'o'})
			m = updated.(Model)

			assert.Equal(t, tt.wantLaunched, *launched)
			view := stripANSI(m.View().Content)
			if tt.wantLaunched {
				assert.Equal(t, tt.wantName, *gotName)
				assert.Equal(t, tt.wantArgs, *gotArgs,
					"workspace path should be the final argument")
				assert.Contains(t, view, "Opened "+workspacePath)
			} else {
				assert.Contains(t, view, "No editor found")
			}
		})
	}
}

func TestOpenEditorNoWorkspaceRecorded(t *testing.T) {
	t.Setenv("VISUAL", "myvisual")
	t.Setenv("EDITOR", "")
	launched, _, _ := stubEditorEnv(t, false)

	m := setupModelWithAgents(1)

	updated, _ := m.Update(tea.KeyPressMsg{Text: "o", Code: 'o'})
	m = updated.(Model)

	assert.False(t, *launched, "launcher must not run without a workspace path")
	assert.Contains(t, stripANSI(m.View().Content), "No workspace recorded")
}

func TestOpenEditorNoSelection(t *testing.T) {
	t.Setenv("VISUAL", "myvisual")
	t.Setenv("EDITOR", "")
	launched, _, _ := stubEditorEnv(t, false)

	m := NewModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Text: "o", Code: 'o'})
	m = updated.(Model)

	assert.False(t, *launched)
	assert.Contains(t, stripANSI(m.View().Content), "No running issue selected")
}

func TestOpenEditorUsesWorkspaceFromAgentStarted(t *testing.T) {
	t.Setenv("VISUAL", "myvisual")
	t.Setenv("EDITOR", "")
	launched, gotName, gotArgs := stubEditorEnv(t, false)

	m := NewModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(OrchestratorEventMsg{Event: orchestrator.OrchestratorEvent{
		Type:      orchestrator.EventAgentStarted,
		IssueID:   "ISSUE-1",
		Timestamp: time.Now(),
		Data: orchestrator.AgentStarted{
			Attempt:   1,
			PID:       100,
			SessionID: "sess-1",
			Workspace: `C:\worktrees\issue-1`,
		},
	}})
	m = updated.(Model)
	require.Equal(t, `C:\worktrees\issue-1`, m.agentWorkspaces["ISSUE-1"],
		"workspace should be captured from AgentStarted")

	updated, _ = m.Update(tea.KeyPressMsg{Text: "o", Code: 'o'})
	m = updated.(Model)

	assert.True(t, *launched)
	assert.Equal(t, "myvisual", *gotName)
	assert.Equal(t, []string{`C:\worktrees\issue-1`}, *gotArgs)
}

func TestStatusMessageExpiresOnTick(t *testing.T) {
	m := setupModelWithAgents(1)
	m = m.withStatus("transient notice")
	assert.Contains(t, stripANSI(m.View().Content), "transient notice")

	// Not yet expired: tick keeps the message.
	updated, _ := m.Update(tickMsg(time.Now()))
	m = updated.(Model)
	assert.Contains(t, stripANSI(m.View().Content), "transient notice")

	// Force expiry, then tick clears it and the help line returns.
	m.statusExpires = time.Now().Add(-time.Second)
	updated, _ = m.Update(tickMsg(time.Now()))
	m = updated.(Model)
	view := stripANSI(m.View().Content)
	assert.NotContains(t, view, "transient notice")
	assert.Contains(t, view, "q quit")
}

func TestHelpLineMentionsTranscriptAndEditorKeys(t *testing.T) {
	m := NewModel()
	stripped := stripANSI(m.help.View(m.keys))
	assert.Contains(t, stripped, "t transcript")
	assert.Contains(t, stripped, "o editor")
}
