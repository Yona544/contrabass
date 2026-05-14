package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/junhoyeo/contrabass/internal/agent"
	"github.com/junhoyeo/contrabass/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowHooksRunAtAgentLifecycleBoundaries(t *testing.T) {
	baseDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "hooks.log")
	t.Setenv("CONTRABASS_HOOK_LOG", logPath)
	recorder := writeLifecycleHookRecorder(t)

	issue := types.Issue{ID: "ISS-HOOK", Identifier: "CB-42", Title: "Hook test", State: types.Unclaimed}
	mt := newObservingTracker([]types.Issue{issue})
	ws := newDirectoryWorkspace(baseDir)
	runner := &agent.MockRunner{
		Events: []types.AgentEvent{{Type: "turn/completed"}},
		Delay:  10 * time.Millisecond,
	}

	cfg := testConfig()
	cfg.Hooks.BeforeRun = hookCommand(recorder, "before")
	cfg.Hooks.AfterRun = hookCommand(recorder, "after")
	orch := NewOrchestrator(mt, ws, runner, &staticConfig{cfg: cfg}, nil)
	go func() {
		for range orch.Events() {
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := startOrchestrator(ctx, orch)

	require.Eventually(t, func() bool {
		return mt.ReleaseCount(issue.ID) > 0
	}, 8*time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-done)

	workspacePath := filepath.Join(baseDir, "workspaces", issue.ID)
	assert.Equal(t, []string{
		"before|ISS-HOOK|CB-42|" + workspacePath,
		"after|ISS-HOOK|CB-42|" + workspacePath,
	}, readHookLog(t, logPath))
	assert.False(t, ws.Exists(issue.ID), "workspace should be cleaned after after_run hook")
}

func TestBeforeRunHookFailureSkipsAgentStartAndQueuesRetry(t *testing.T) {
	failHook := writeFailingLifecycleHook(t)
	issue := types.Issue{ID: "ISS-HOOK-FAIL", Identifier: "CB-43", Title: "Hook failure", State: types.Unclaimed}
	mt := newObservingTracker([]types.Issue{issue})
	ws := newDirectoryWorkspace(t.TempDir())
	runner := newTrackingRunner(&agent.MockRunner{
		Events: []types.AgentEvent{{Type: "turn/completed"}},
	})

	cfg := testConfig()
	cfg.Hooks.BeforeRun = hookCommand(failHook)
	orch := NewOrchestrator(mt, ws, runner, &staticConfig{cfg: cfg}, nil)
	go func() {
		for range orch.Events() {
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := startOrchestrator(ctx, orch)

	require.Eventually(t, func() bool {
		return mt.ReleaseCount(issue.ID) > 0 && len(backoffSnapshot(orch)) > 0
	}, 8*time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-done)

	require.Equal(t, 0, runner.StartCount(), "agent must not start when before_run hook fails")
	assert.False(t, ws.Exists(issue.ID), "failed before_run hook should clean the prepared workspace")
	require.Len(t, backoffSnapshot(orch), 1)
	assert.Contains(t, backoffSnapshot(orch)[0].Error, "before_run hook failed")
}

type directoryWorkspace struct {
	baseDir string
}

func newDirectoryWorkspace(baseDir string) *directoryWorkspace {
	return &directoryWorkspace{baseDir: baseDir}
}

func (w *directoryWorkspace) Create(_ context.Context, issue types.Issue) (string, error) {
	workspacePath := filepath.Join(w.baseDir, "workspaces", issue.ID)
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		return "", err
	}
	return workspacePath, nil
}

func (w *directoryWorkspace) Cleanup(_ context.Context, issueID string) error {
	return os.RemoveAll(filepath.Join(w.baseDir, "workspaces", issueID))
}

func (w *directoryWorkspace) CleanupAll(_ context.Context) error {
	return os.RemoveAll(filepath.Join(w.baseDir, "workspaces"))
}

func (w *directoryWorkspace) Exists(issueID string) bool {
	info, err := os.Stat(filepath.Join(w.baseDir, "workspaces", issueID))
	return err == nil && info.IsDir()
}

func writeLifecycleHookRecorder(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "record-hook.cmd")
		content := "@echo off\r\n" +
			">> \"%CONTRABASS_HOOK_LOG%\" echo %1^|%CONTRABASS_ISSUE_ID%^|%CONTRABASS_ISSUE_IDENTIFIER%^|%CONTRABASS_WORKSPACE%\r\n"
		require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
		return path
	}

	path := filepath.Join(dir, "record-hook.sh")
	content := "#!/bin/sh\n" +
		"printf '%s|%s|%s|%s\\n' \"$1\" \"$CONTRABASS_ISSUE_ID\" \"$CONTRABASS_ISSUE_IDENTIFIER\" \"$CONTRABASS_WORKSPACE\" >> \"$CONTRABASS_HOOK_LOG\"\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
	return path
}

func writeFailingLifecycleHook(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "fail-hook.cmd")
		require.NoError(t, os.WriteFile(path, []byte("@echo off\r\necho hook failed 1>&2\r\nexit /b 9\r\n"), 0o755))
		return path
	}

	path := filepath.Join(dir, "fail-hook.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho hook failed >&2\nexit 9\n"), 0o755))
	return path
}

func hookCommand(parts ...string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, quoteHookCommandPart(part))
	}
	return strings.Join(quoted, " ")
}

func quoteHookCommandPart(part string) string {
	if runtime.GOOS == "windows" && !strings.ContainsAny(part, " \t\"") {
		return part
	}
	escaped := strings.ReplaceAll(part, `"`, `\"`)
	return `"` + escaped + `"`
}

func readHookLog(t *testing.T, path string) []string {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
}
