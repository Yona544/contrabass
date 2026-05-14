package workspace

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/junhoyeo/contrabass/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerCleanupRunsBeforeRemoveHook(t *testing.T) {
	repoDir := initGitRepo(t)
	logPath := filepath.Join(t.TempDir(), "before-remove.log")
	t.Setenv("CONTRABASS_HOOK_LOG", logPath)

	mgr := NewManager(repoDir)
	mgr.SetBeforeRemoveHook(workspaceHookCommand(writeWorkspaceHookRecorder(t)))
	ctx := context.Background()

	issue := types.Issue{ID: "ISS-BEFORE-REMOVE"}
	workspacePath, err := mgr.Create(ctx, issue)
	require.NoError(t, err)

	require.NoError(t, mgr.Cleanup(ctx, issue.ID))

	assert.Equal(t, []string{
		"ISS-BEFORE-REMOVE|" + workspacePath,
	}, readWorkspaceHookLog(t, logPath))
	assert.False(t, mgr.Exists(issue.ID))
	assert.NoDirExists(t, workspacePath)
}

func TestManagerCleanupContinuesWhenBeforeRemoveHookFails(t *testing.T) {
	repoDir := initGitRepo(t)
	mgr := NewManager(repoDir)
	mgr.SetBeforeRemoveHook(workspaceHookCommand(writeFailingWorkspaceHook(t)))
	ctx := context.Background()

	issue := types.Issue{ID: "ISS-BEFORE-REMOVE-FAIL"}
	workspacePath, err := mgr.Create(ctx, issue)
	require.NoError(t, err)

	require.NoError(t, mgr.Cleanup(ctx, issue.ID))

	assert.False(t, mgr.Exists(issue.ID))
	assert.NoDirExists(t, workspacePath)
}

func writeWorkspaceHookRecorder(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "record-before-remove.cmd")
		content := "@echo off\r\n" +
			">> \"%CONTRABASS_HOOK_LOG%\" echo %CONTRABASS_ISSUE_ID%^|%CONTRABASS_WORKSPACE%\r\n"
		require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
		return path
	}

	path := filepath.Join(dir, "record-before-remove.sh")
	content := "#!/bin/sh\n" +
		"printf '%s|%s\\n' \"$CONTRABASS_ISSUE_ID\" \"$CONTRABASS_WORKSPACE\" >> \"$CONTRABASS_HOOK_LOG\"\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
	return path
}

func writeFailingWorkspaceHook(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "fail-before-remove.cmd")
		require.NoError(t, os.WriteFile(path, []byte("@echo off\r\necho hook failed 1>&2\r\nexit /b 9\r\n"), 0o755))
		return path
	}

	path := filepath.Join(dir, "fail-before-remove.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho hook failed >&2\nexit 9\n"), 0o755))
	return path
}

func workspaceHookCommand(parts ...string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, workspaceQuoteHookPart(part))
	}
	return strings.Join(quoted, " ")
}

func workspaceQuoteHookPart(part string) string {
	if runtime.GOOS == "windows" && !strings.ContainsAny(part, " \t\"") {
		return part
	}
	escaped := strings.ReplaceAll(part, `"`, `\"`)
	return `"` + escaped + `"`
}

func readWorkspaceHookLog(t *testing.T, path string) []string {
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
