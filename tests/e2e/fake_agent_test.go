package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/agent"
	"github.com/junhoyeo/contrabass/internal/types"
)

func TestFakeAgentCodexProtocol(t *testing.T) {
	root := projectRoot(t)
	scriptPath := filepath.Join(root, "testdata", "mock-agent.sh")

	_, err := os.Stat(scriptPath)
	require.NoError(t, err)

	t.Setenv("MOCK_AGENT_DELAY", "0")

	binaryPath := "bash " + scriptPath
	if runtime.GOOS == "windows" {
		binaryPath = fakeAgentHelperCommand(t)
	}
	runner := agent.NewCodexRunner(binaryPath, 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	issue := types.Issue{ID: "E2E-FAKE-1", Title: "fake agent protocol"}

	proc, err := runner.Start(ctx, issue, tmpDir, "test prompt")
	require.NoError(t, err)

	defer func() {
		stopErr := runner.Stop(proc)
		if stopErr != nil {
			require.True(t, strings.Contains(stopErr.Error(), "already stopped"), "unexpected stop error: %v", stopErr)
		}
	}()

	select {
	case doneErr := <-proc.Done:
		require.NoError(t, doneErr)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for fake agent process completion")
	}

	events := drainAgentEvents(proc.Events)
	assert.NotEmpty(t, events)
}

func fakeAgentHelperCommand(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "mock-agent.go")
	exePath := filepath.Join(dir, "mock-agent.exe")
	src := `package main

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

func main() {
	delay := time.Second
	if os.Getenv("MOCK_AGENT_DELAY") == "0" {
		delay = 0
	}

	scanner := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	for scanner.Scan() {
		var msg map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		id := msg["id"]
		method, _ := msg["method"].(string)
		switch method {
		case "initialize":
			writeJSON(writer, map[string]interface{}{"jsonrpc": "2.0", "id": id, "result": map[string]interface{}{"capabilities": map[string]interface{}{}}})
		case "initialized":
		case "thread/start":
			writeJSON(writer, map[string]interface{}{"jsonrpc": "2.0", "id": id, "result": map[string]interface{}{"thread": map[string]interface{}{"id": "mock-thread"}}})
		case "turn/start":
			writeJSON(writer, map[string]interface{}{"jsonrpc": "2.0", "id": id, "result": map[string]interface{}{"turn": map[string]interface{}{"id": "mock-turn"}}})
			time.Sleep(delay)
			writeJSON(writer, map[string]interface{}{"jsonrpc": "2.0", "method": "item/created", "params": map[string]interface{}{"type": "message", "content": "Analyzing the task..."}})
			time.Sleep(delay)
			writeJSON(writer, map[string]interface{}{"jsonrpc": "2.0", "method": "item/completed", "params": map[string]interface{}{"type": "message", "content": "Task completed successfully."}})
			writeJSON(writer, map[string]interface{}{"jsonrpc": "2.0", "method": "turn/completed", "params": map[string]interface{}{}})
			return
		}
	}
}

func writeJSON(writer *bufio.Writer, v map[string]interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		os.Exit(1)
	}
	_, _ = writer.Write(append(data, '\n'))
	_ = writer.Flush()
}
`
	require.NoError(t, os.WriteFile(srcPath, []byte(src), 0o644))
	cmd := exec.Command("go", "build", "-o", exePath, srcPath)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "build mock agent helper: %s", output)
	return exePath
}

func projectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(wd, "..", "..")
}

func drainAgentEvents(events <-chan types.AgentEvent) []types.AgentEvent {
	out := make([]types.AgentEvent, 0)
	for evt := range events {
		out = append(out, evt)
	}
	return out
}
