package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/junhoyeo/contrabass/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func claudeHelperCommand(t *testing.T, mode string) string {
	t.Helper()

	exe := copyCurrentTestExecutable(t, "mock-claude-helper")
	return fmt.Sprintf("%s -test.run=TestClaudeCodeHelperProcess -- %s", exe, mode)
}

func newClaudeHelperRunner(t *testing.T, mode string, cfg ClaudeCodeConfig) *ClaudeCodeRunner {
	t.Helper()

	cfg.BinaryPath = claudeHelperCommand(t, mode)
	return NewClaudeCodeRunner(cfg)
}

func TestClaudeCodeRunner_DefaultBinaryPath(t *testing.T) {
	tests := []struct {
		name       string
		binaryPath string
	}{
		{"empty string", ""},
		{"whitespace only", "   \t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewClaudeCodeRunner(ClaudeCodeConfig{BinaryPath: tt.binaryPath})
			assert.Equal(t, defaultClaudeCodeBinaryPath, runner.cfg.BinaryPath)
		})
	}
}

func TestClaudeCodeRunner_BuildArgs(t *testing.T) {
	tests := []struct {
		name string
		cfg  ClaudeCodeConfig
		want []string
	}{
		{
			name: "defaults omit optional flags",
			cfg:  ClaudeCodeConfig{},
			want: []string{"-p", "--output-format", "stream-json", "--verbose"},
		},
		{
			name: "whitespace-only fields are skipped",
			cfg:  ClaudeCodeConfig{Model: "  ", PermissionMode: "\t"},
			want: []string{"-p", "--output-format", "stream-json", "--verbose"},
		},
		{
			name: "all options rendered in canonical order",
			cfg: ClaudeCodeConfig{
				Model:          "claude-sonnet-4-5",
				PermissionMode: "bypassPermissions",
				MaxTurns:       5,
				AllowedTools:   []string{"Bash", "Read"},
				ExtraArgs:      []string{"--add-dir", "/extra"},
			},
			want: []string{
				"-p", "--output-format", "stream-json", "--verbose",
				"--model", "claude-sonnet-4-5",
				"--permission-mode", "bypassPermissions",
				"--max-turns", "5",
				"--allowedTools", "Bash,Read",
				"--add-dir", "/extra",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewClaudeCodeRunner(tt.cfg)
			assert.Equal(t, tt.want, runner.buildArgs())
		})
	}
}

func TestClaudeCodeRunner_SuccessfulRun(t *testing.T) {
	runner := newClaudeHelperRunner(t, "success", ClaudeCodeConfig{})

	proc, err := runner.Start(context.Background(), types.Issue{ID: "MT-1", Title: "Task 1"}, t.TempDir(), "hello")
	require.NoError(t, err)
	assert.Greater(t, proc.PID, 0)
	assert.Equal(t, "sess-success-1", proc.SessionID)

	events := collectEvents(t, proc.Events, proc.Done, 6, 5*time.Second)
	require.Len(t, events, 6)

	typesSeen := make([]string, 0, len(events))
	for _, event := range events {
		typesSeen = append(typesSeen, event.Type)
		assert.False(t, event.Timestamp.IsZero())
	}
	assert.Equal(t, []string{
		"system/init",
		"item/agentMessage",
		"item/toolCall",
		"session.usage",
		"item/toolResult",
		"turn/completed",
	}, typesSeen)

	assert.Equal(t, "working on it", events[1].Data["text"])
	assert.Equal(t, "Bash", events[2].Data["name"])

	interim := eventDataMap(t, events[3], "usage")
	assert.Equal(t, float64(10), interim["input_tokens"])
	assert.Equal(t, float64(3), interim["output_tokens"])

	terminal := events[5]
	assert.Equal(t, "sess-success-1", terminal.Data["session_id"])
	assert.Equal(t, float64(2), terminal.Data["num_turns"])
	assert.Equal(t, 0.0042, terminal.Data["total_cost_usd"])
	usage := eventDataMap(t, terminal, "usage")
	assert.Equal(t, float64(42), usage["input_tokens"])
	assert.Equal(t, float64(7), usage["output_tokens"])

	select {
	case doneErr := <-proc.Done:
		assert.NoError(t, doneErr)
	case <-time.After(5 * time.Second):
		t.Fatal("expected done channel to be signaled")
	}
}

func TestClaudeCodeRunner_FlagAndPromptPropagation(t *testing.T) {
	runner := newClaudeHelperRunner(t, "capture-args", ClaudeCodeConfig{
		Model:          "claude-sonnet-4-5",
		PermissionMode: "bypassPermissions",
		MaxTurns:       5,
		AllowedTools:   []string{"Bash", "Read"},
		ExtraArgs:      []string{"--add-dir", "/extra"},
	})

	proc, err := runner.Start(context.Background(), types.Issue{ID: "MT-2", Title: "Task 2"}, t.TempDir(), "build the thing please")
	require.NoError(t, err)
	assert.Equal(t, "sess-args-1", proc.SessionID)

	event := waitForEventType(t, proc.Events, "helper/args")

	rawArgs, ok := event.Data["args"].([]interface{})
	require.True(t, ok)
	args := make([]string, 0, len(rawArgs))
	for _, arg := range rawArgs {
		args = append(args, fmt.Sprint(arg))
	}
	assert.Equal(t, []string{
		"-p", "--output-format", "stream-json", "--verbose",
		"--model", "claude-sonnet-4-5",
		"--permission-mode", "bypassPermissions",
		"--max-turns", "5",
		"--allowedTools", "Bash,Read",
		"--add-dir", "/extra",
	}, args)

	assert.Equal(t, "build the thing please", event.Data["prompt"])
	assertDoneEventually(t, proc.Done)
}

func TestClaudeCodeRunner_ResultError(t *testing.T) {
	runner := newClaudeHelperRunner(t, "fail", ClaudeCodeConfig{})

	proc, err := runner.Start(context.Background(), types.Issue{ID: "MT-3", Title: "Task 3"}, t.TempDir(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "sess-fail-1", proc.SessionID)

	event := waitForEventType(t, proc.Events, "turn/failed")
	assert.Equal(t, "boom failed", event.Data["error"])
	assert.Equal(t, "error_during_execution", event.Data["subtype"])
	usage := eventDataMap(t, event, "usage")
	assert.Equal(t, float64(5), usage["input_tokens"])

	select {
	case doneErr := <-proc.Done:
		require.Error(t, doneErr)
		assert.ErrorIs(t, doneErr, errClaudeCodeRunFailed)
		assert.Contains(t, doneErr.Error(), "boom failed")
	case <-time.After(5 * time.Second):
		t.Fatal("expected done channel to report the result error")
	}
}

func TestClaudeCodeRunner_NonzeroExitFailure(t *testing.T) {
	runner := newClaudeHelperRunner(t, "exit-error", ClaudeCodeConfig{})

	proc, err := runner.Start(context.Background(), types.Issue{ID: "MT-4", Title: "Task 4"}, t.TempDir(), "hello")
	require.NoError(t, err)

	select {
	case doneErr := <-proc.Done:
		require.Error(t, doneErr)
		assert.Contains(t, doneErr.Error(), "exit status 3")
		assert.Contains(t, doneErr.Error(), "stderr: claude stderr exploded")
	case <-time.After(5 * time.Second):
		t.Fatal("expected done channel to report the exit error")
	}
}

func TestClaudeCodeRunner_MalformedJSONSkipped(t *testing.T) {
	runner := newClaudeHelperRunner(t, "malformed", ClaudeCodeConfig{})

	proc, err := runner.Start(context.Background(), types.Issue{ID: "MT-5", Title: "Task 5"}, t.TempDir(), "hello")
	require.NoError(t, err)

	events := collectEvents(t, proc.Events, proc.Done, 3, 5*time.Second)
	require.Len(t, events, 3)

	assert.Equal(t, "system/init", events[0].Type)

	assert.Equal(t, "protocol/error", events[1].Type)
	errMsg, ok := events[1].Data["error"].(string)
	require.True(t, ok)
	assert.Contains(t, errMsg, "malformed JSON")
	raw, ok := events[1].Data["raw"].(string)
	require.True(t, ok)
	assert.Contains(t, raw, "this is not json")

	assert.Equal(t, "turn/completed", events[2].Type)

	select {
	case doneErr := <-proc.Done:
		assert.NoError(t, doneErr)
	case <-time.After(5 * time.Second):
		t.Fatal("expected process to exit normally")
	}
}

func TestClaudeCodeRunner_StallTimeout(t *testing.T) {
	streamTimeout := 150 * time.Millisecond
	runner := newClaudeHelperRunner(t, "stall", ClaudeCodeConfig{StreamReadTimeout: streamTimeout})

	proc, err := runner.Start(context.Background(), types.Issue{ID: "MT-6", Title: "Task 6"}, t.TempDir(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "sess-stall-1", proc.SessionID)

	start := time.Now()
	select {
	case doneErr := <-proc.Done:
		elapsed := time.Since(start)
		require.Error(t, doneErr)
		assert.ErrorIs(t, doneErr, errClaudeCodeStreamStalled)
		assert.GreaterOrEqual(t, elapsed, streamTimeout)
		assert.Less(t, elapsed, 5*time.Second)
	case <-time.After(5 * time.Second):
		t.Fatal("expected stream read timeout")
	}
}

func TestClaudeCodeRunner_StartFailsWhenSilent(t *testing.T) {
	runner := newClaudeHelperRunner(t, "silent", ClaudeCodeConfig{StreamReadTimeout: 50 * time.Millisecond})
	runner.initTimeout = 100 * time.Millisecond

	proc, err := runner.Start(context.Background(), types.Issue{ID: "MT-7", Title: "Task 7"}, t.TempDir(), "hello")
	require.Error(t, err)
	assert.Nil(t, proc)
	assert.ErrorIs(t, err, errClaudeCodeStreamStalled)
	assert.Contains(t, err.Error(), "waiting for init event")

	runner.mu.Lock()
	assert.Empty(t, runner.procs)
	runner.mu.Unlock()
}

func TestClaudeCodeRunner_ContextCancellationKillsProcess(t *testing.T) {
	runner := newClaudeHelperRunner(t, "stall", ClaudeCodeConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proc, err := runner.Start(ctx, types.Issue{ID: "MT-8", Title: "Task 8"}, t.TempDir(), "hello")
	require.NoError(t, err)

	cancel()

	select {
	case doneErr := <-proc.Done:
		require.Error(t, doneErr)
		assert.ErrorIs(t, doneErr, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("expected cancellation to terminate the process")
	}
}

func TestClaudeCodeRunner_StopTerminatesProcess(t *testing.T) {
	runner := newClaudeHelperRunner(t, "hang", ClaudeCodeConfig{})

	proc, err := runner.Start(context.Background(), types.Issue{ID: "MT-9", Title: "Task 9"}, t.TempDir(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "sess-hang-1", proc.SessionID)

	require.NoError(t, runner.Stop(proc))
	assertDoneEventually(t, proc.Done)

	assert.Eventually(t, func() bool {
		runner.mu.Lock()
		defer runner.mu.Unlock()
		return len(runner.procs) == 0
	}, 2*time.Second, 10*time.Millisecond, "expected process bookkeeping to be released")
}

func TestClaudeCodeRunner_StopNilProcess(t *testing.T) {
	runner := NewClaudeCodeRunner(ClaudeCodeConfig{BinaryPath: "/nonexistent"})
	err := runner.Stop(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "process is nil")
}

func TestClaudeCodeRunner_StopUnknownProcess(t *testing.T) {
	runner := NewClaudeCodeRunner(ClaudeCodeConfig{BinaryPath: "/nonexistent"})
	err := runner.Stop(&AgentProcess{PID: 424242})
	require.Error(t, err)
	assert.ErrorIs(t, err, errClaudeCodeAlreadyStopped)
}

func TestClaudeCodeRunner_Close(t *testing.T) {
	runner := NewClaudeCodeRunner(ClaudeCodeConfig{})
	assert.NoError(t, runner.Close())
}

// TestClaudeCodeHelperProcess is not a real test: it acts as the fake
// `claude` binary for the ClaudeCodeRunner tests, mirroring the
// TestCodexHelperProcess technique. The mode is passed after a `--` arg and
// selects the stream-json script the helper plays back on stdout.
func TestClaudeCodeHelperProcess(t *testing.T) {
	args := argsAfterDoubleDash(os.Args)
	mode := ""
	if len(args) > 0 {
		mode = args[0]
	}
	if mode == "" {
		return
	}

	prompt, _ := io.ReadAll(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)

	emitInit := func(sessionID string) {
		writeJSON(t, writer, map[string]interface{}{
			"type":       "system",
			"subtype":    "init",
			"session_id": sessionID,
			"model":      "claude-test",
		})
	}
	emitResultSuccess := func(sessionID string) {
		writeJSON(t, writer, map[string]interface{}{
			"type":           "result",
			"subtype":        "success",
			"is_error":       false,
			"result":         "done",
			"num_turns":      2,
			"total_cost_usd": 0.0042,
			"usage":          map[string]interface{}{"input_tokens": 42, "output_tokens": 7},
			"session_id":     sessionID,
		})
	}

	switch mode {
	case "success":
		emitInit("sess-success-1")
		writeJSON(t, writer, map[string]interface{}{
			"type": "assistant",
			"message": map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "working on it"},
					map[string]interface{}{"type": "tool_use", "id": "toolu_1", "name": "Bash", "input": map[string]interface{}{"command": "ls"}},
				},
				"usage": map[string]interface{}{"input_tokens": 10, "output_tokens": 3},
			},
			"session_id": "sess-success-1",
		})
		writeJSON(t, writer, map[string]interface{}{
			"type": "user",
			"message": map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{"type": "tool_result", "tool_use_id": "toolu_1", "content": "ok"},
				},
			},
			"session_id": "sess-success-1",
		})
		emitResultSuccess("sess-success-1")
	case "capture-args":
		emitInit("sess-args-1")
		writeJSON(t, writer, map[string]interface{}{
			"type":       "helper/args",
			"args":       args[1:],
			"prompt":     string(prompt),
			"session_id": "sess-args-1",
		})
		emitResultSuccess("sess-args-1")
	case "fail":
		emitInit("sess-fail-1")
		writeJSON(t, writer, map[string]interface{}{
			"type":       "result",
			"subtype":    "error_during_execution",
			"is_error":   true,
			"result":     "boom failed",
			"num_turns":  1,
			"usage":      map[string]interface{}{"input_tokens": 5, "output_tokens": 1},
			"session_id": "sess-fail-1",
		})
		os.Exit(1)
	case "exit-error":
		emitInit("sess-exit-1")
		_, _ = fmt.Fprintln(os.Stderr, "claude stderr exploded")
		os.Exit(3)
	case "malformed":
		emitInit("sess-mal-1")
		_, err := writer.WriteString("this is not json\n")
		require.NoError(t, err)
		require.NoError(t, writer.Flush())
		emitResultSuccess("sess-mal-1")
	case "stall":
		emitInit("sess-stall-1")
		time.Sleep(10 * time.Second)
	case "silent":
		time.Sleep(10 * time.Second)
	case "hang":
		emitInit("sess-hang-1")
		time.Sleep(30 * time.Second)
	default:
		os.Exit(9)
	}
}
