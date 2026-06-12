package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/tracker"
)

const planTestArray = `[
  {"title": "Add schema", "description": "Create the users table.\nInclude indexes.", "blocked_by": []},
  {"title": "Add API", "description": "Expose REST endpoints for users.", "blocked_by": [0]},
  {"title": "Add UI", "description": "Build the users page.", "blocked_by": [0, 1]}
]`

func restorePlanExecutor(t *testing.T) {
	t.Helper()

	original := planExecutor
	t.Cleanup(func() { planExecutor = original })
}

func runPlanCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := newPlanCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// planEnvelope wraps model text in the claude -p --output-format json shape.
func planEnvelope(t *testing.T, result string) string {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"type":     "result",
		"is_error": false,
		"result":   result,
	})
	require.NoError(t, err)
	return string(payload)
}

func stubPlanExecutor(t *testing.T, output string, err error) {
	t.Helper()

	restorePlanExecutor(t)
	planExecutor = func(_ context.Context, _ string, _ []string, _ string) (string, error) {
		return output, err
	}
}

func planTestBoardDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "board")
}

func TestPlanPreviewPrintsDependencyTree(t *testing.T) {
	fenced := "Here is the plan:\n```json\n" + planTestArray + "\n```\n"
	stubPlanExecutor(t, planEnvelope(t, fenced), nil)

	boardDir := planTestBoardDir(t)
	stdout, err := runPlanCmd(t, "Build a users feature", "--board-dir", boardDir)
	require.NoError(t, err)

	assert.Contains(t, stdout, "plan for: Build a users feature")
	assert.Contains(t, stdout, "1. Add schema\n")
	assert.Contains(t, stdout, "   Create the users table.")
	assert.NotContains(t, stdout, "Include indexes.")
	assert.Contains(t, stdout, "2. Add API (blocked by: #1)")
	assert.Contains(t, stdout, "3. Add UI (blocked by: #1, #2)")
	assert.Contains(t, stdout, "run again with --apply to create")

	_, statErr := os.Stat(boardDir)
	assert.True(t, os.IsNotExist(statErr), "preview must not create the board directory")
}

func TestPlanApplyCreatesIssuesWithBlockedByWiring(t *testing.T) {
	fenced := "```json\n" + planTestArray + "\n```"
	stubPlanExecutor(t, planEnvelope(t, fenced), nil)

	boardDir := planTestBoardDir(t)
	stdout, err := runPlanCmd(t, "Build a users feature", "--board-dir", boardDir, "--apply")
	require.NoError(t, err)
	assert.Contains(t, stdout, "created 3 issue(s)")
	assert.NotContains(t, stdout, "run again with --apply")

	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{BoardDir: boardDir})
	issues, err := localTracker.ListIssues(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, issues, 3)

	assert.Equal(t, "Add schema", issues[0].Title)
	assert.Equal(t, "Add API", issues[1].Title)
	assert.Equal(t, "Add UI", issues[2].Title)

	assert.Empty(t, issues[0].BlockedBy)
	assert.Equal(t, []string{issues[0].ID}, issues[1].BlockedBy)
	assert.Equal(t, []string{issues[0].ID, issues[1].ID}, issues[2].BlockedBy)

	for _, issue := range issues {
		assert.Equal(t, []string{"planned"}, issue.Labels)
		assert.Contains(t, stdout, "created "+issue.ID)
	}

	last, err := localTracker.GetIssue(context.Background(), issues[2].ID)
	require.NoError(t, err)
	assert.Equal(t, "Build the users page.", last.Description)
	assert.Contains(t, stdout, fmt.Sprintf("created %s Add UI (blocked by %s, %s)", last.ID, issues[0].ID, issues[1].ID))
}

func TestPlanAcceptsBareArrayOutput(t *testing.T) {
	stubPlanExecutor(t, planTestArray, nil)

	stdout, err := runPlanCmd(t, "Build a users feature", "--board-dir", planTestBoardDir(t))
	require.NoError(t, err)
	assert.Contains(t, stdout, "1. Add schema")
	assert.Contains(t, stdout, "3. Add UI (blocked by: #1, #2)")
}

func TestPlanRejectsMalformedOutput(t *testing.T) {
	cases := []struct {
		name    string
		output  string
		wantErr string
	}{
		{
			name:    "prose without array",
			output:  `{"type":"result","result":"no plan today"}`,
			wantErr: "does not contain a JSON array",
		},
		{
			name:    "broken array json",
			output:  `{"type":"result","result":"[{\"title\": }]"}`,
			wantErr: "parsing plan JSON array",
		},
		{
			name:    "truncated envelope",
			output:  `{"type":"result","result":`,
			wantErr: "parsing claude output envelope",
		},
		{
			name:    "empty result field",
			output:  `{"type":"result","result":""}`,
			wantErr: "empty result field",
		},
		{
			name:    "empty output",
			output:  "",
			wantErr: "no output",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubPlanExecutor(t, tc.output, nil)

			_, err := runPlanCmd(t, "task", "--board-dir", planTestBoardDir(t))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestPlanRejectsInvalidBlockedByIndices(t *testing.T) {
	cases := []struct {
		name  string
		array string
	}{
		{
			name:  "forward reference",
			array: `[{"title": "a", "description": "", "blocked_by": [1]}, {"title": "b", "description": "", "blocked_by": []}]`,
		},
		{
			name:  "self reference",
			array: `[{"title": "a", "description": "", "blocked_by": []}, {"title": "b", "description": "", "blocked_by": [1]}]`,
		},
		{
			name:  "out of range",
			array: `[{"title": "a", "description": "", "blocked_by": []}, {"title": "b", "description": "", "blocked_by": [5]}]`,
		},
		{
			name:  "negative index",
			array: `[{"title": "a", "description": "", "blocked_by": []}, {"title": "b", "description": "", "blocked_by": [-1]}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubPlanExecutor(t, planEnvelope(t, tc.array), nil)

			_, err := runPlanCmd(t, "task", "--board-dir", planTestBoardDir(t))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid blocked_by index")
		})
	}
}

func TestPlanRejectsTooManyIssues(t *testing.T) {
	stubPlanExecutor(t, planEnvelope(t, planTestArray), nil)

	_, err := runPlanCmd(t, "task", "--board-dir", planTestBoardDir(t), "--max-issues", "2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeding --max-issues 2")
}

func TestPlanRejectsEmptyTitle(t *testing.T) {
	array := `[{"title": "a", "description": "", "blocked_by": []}, {"title": "   ", "description": "x", "blocked_by": []}]`
	stubPlanExecutor(t, planEnvelope(t, array), nil)

	_, err := runPlanCmd(t, "task", "--board-dir", planTestBoardDir(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty title")
}

func TestPlanPropagatesExecutorError(t *testing.T) {
	stubPlanExecutor(t, "", errors.New("claude exploded"))

	_, err := runPlanCmd(t, "task", "--board-dir", planTestBoardDir(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claude exploded")
}

func TestPlanSendsPromptOnStdin(t *testing.T) {
	restorePlanExecutor(t)

	var gotName string
	var gotArgs []string
	var gotStdin string
	planExecutor = func(_ context.Context, name string, args []string, stdin string) (string, error) {
		gotName = name
		gotArgs = args
		gotStdin = stdin
		return planEnvelope(t, planTestArray), nil
	}

	task := "Build a billing page with invoices"
	_, err := runPlanCmd(t, task, "--board-dir", planTestBoardDir(t), "--max-issues", "4")
	require.NoError(t, err)

	assert.Equal(t, "claude", gotName)
	assert.Equal(t, []string{"-p", "--output-format", "json"}, gotArgs[:3])
	assert.Contains(t, gotStdin, task)
	assert.Contains(t, gotStdin, "2 to 4 issues")
	assert.Contains(t, gotStdin, "JSON array")
	assert.NotContains(t, strings.Join(gotArgs, " "), task, "the task must travel on stdin, not argv")
}

func TestPlanUsesConfigBinaryModelAndBoardDir(t *testing.T) {
	restorePlanExecutor(t)

	boardDir := filepath.ToSlash(planTestBoardDir(t))
	workflow := fmt.Sprintf(`---
model: claude-sonnet-4-6
agent:
  type: claude
claude:
  binary_path: claude-test
  model: test-model
tracker:
  type: internal
  board_dir: "%s"
---
{{ issue.description }}
`, boardDir)
	cfgPath := filepath.Join(t.TempDir(), "WORKFLOW.md")
	require.NoError(t, os.WriteFile(cfgPath, []byte(workflow), 0o644))

	var gotName string
	var gotArgs []string
	planExecutor = func(_ context.Context, name string, args []string, _ string) (string, error) {
		gotName = name
		gotArgs = args
		return planEnvelope(t, planTestArray), nil
	}

	stdout, err := runPlanCmd(t, "Build a users feature", "--config", cfgPath, "--apply", "--label", "roadmap")
	require.NoError(t, err)
	assert.Contains(t, stdout, "created 3 issue(s)")

	assert.Equal(t, "claude-test", gotName)
	assert.Equal(t, []string{"-p", "--output-format", "json", "--model", "test-model"}, gotArgs)

	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{BoardDir: boardDir})
	issues, err := localTracker.ListIssues(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, issues, 3)
	for _, issue := range issues {
		assert.Equal(t, []string{"roadmap"}, issue.Labels)
	}
}
