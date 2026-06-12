package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// statusMessageTTL is how long a transient footer status message stays visible.
const statusMessageTTL = 5 * time.Second

// launchEditor starts the resolved editor command detached from the TUI:
// the process is started but never waited on synchronously, and stdout/stderr
// are left nil so they are discarded (connected to the null device).
// It is a package-level var so tests can stub the actual process launch.
var launchEditor = func(name string, args []string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the process in the background so it does not linger as a zombie;
	// the TUI never blocks on it.
	go func() { _ = cmd.Wait() }()
	return nil
}

// editorLookPath resolves a binary on PATH. Injectable for tests.
var editorLookPath = exec.LookPath

// resolveEditor returns the editor command and any leading arguments,
// consulting $VISUAL, then $EDITOR, then `code` if found on PATH.
func resolveEditor() (string, []string, bool) {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		value := strings.TrimSpace(os.Getenv(env))
		if value == "" {
			continue
		}
		fields := strings.Fields(value)
		return fields[0], fields[1:], true
	}
	if _, err := editorLookPath("code"); err == nil {
		return "code", nil, true
	}
	return "", nil, false
}

// openWorkspaceInEditor launches an editor on the workspace directory of the
// currently selected running issue and records a transient status message.
func (m Model) openWorkspaceInEditor() Model {
	issueID := m.selectedIssueID()
	if issueID == "" {
		return m.withStatus("No running issue selected")
	}
	workspace := m.agentWorkspaces[issueID]
	if workspace == "" {
		return m.withStatus(fmt.Sprintf("No workspace recorded for %s", displayIssueID(issueID)))
	}
	name, args, ok := resolveEditor()
	if !ok {
		return m.withStatus("No editor found: set $VISUAL or $EDITOR, or install `code`")
	}
	if err := launchEditor(name, append(args, workspace)); err != nil {
		return m.withStatus(fmt.Sprintf("Editor launch failed: %v", err))
	}
	return m.withStatus(fmt.Sprintf("Opened %s with %s", workspace, name))
}

// withStatus sets a transient footer status message.
func (m Model) withStatus(message string) Model {
	m.statusMessage = message
	m.statusExpires = time.Now().Add(statusMessageTTL)
	return m
}
