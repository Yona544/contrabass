package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/junhoyeo/contrabass/internal/types"
)

const (
	defaultClaudeCodeBinaryPath  = "claude"
	defaultClaudeCodeInitTimeout = 30 * time.Second
	defaultClaudeCodeStopGrace   = 5 * time.Second
	maxClaudeCodeStderrBytes     = 64 * 1024
)

var (
	errClaudeCodeAlreadyStopped = errors.New("claude process already stopped")
	errClaudeCodeStopFailed     = errors.New("claude process stop failed")
	errClaudeCodeStreamStalled  = errors.New("claude stream read timeout")
	errClaudeCodeRunFailed      = errors.New("claude run failed")
)

var terminalClaudeCodeEventTypes = map[string]struct{}{
	"turn/completed": {},
	"turn/failed":    {},
}

// ClaudeCodeConfig captures workflow-driven settings that contrabass forwards
// to the `claude` CLI as command-line flags. Zero values are skipped so claude
// falls back to whatever is configured in the operator's own settings.
// PermissionMode is passed through verbatim; callers that want unattended
// runs typically set it to "bypassPermissions" — contrabass never injects it
// implicitly.
type ClaudeCodeConfig struct {
	BinaryPath        string
	Model             string
	PermissionMode    string
	MaxTurns          int
	AllowedTools      []string
	ExtraArgs         []string
	StreamReadTimeout time.Duration
}

// ClaudeCodeRunner drives the `claude` CLI in headless print mode
// (`claude -p --output-format stream-json --verbose`), one subprocess per
// issue run. The prompt travels via stdin to avoid argument-length limits on
// Windows; stream-json events on stdout are translated into the codex-style
// AgentEvent taxonomy the orchestrator already understands (turn/completed,
// turn/failed, item/agentMessage, session.usage, protocol/error).
type ClaudeCodeRunner struct {
	cfg         ClaudeCodeConfig
	initTimeout time.Duration
	stopGrace   time.Duration
	logger      *log.Logger

	mu    sync.Mutex
	procs map[int]*claudeCodeProcess
}

var _ AgentRunner = (*ClaudeCodeRunner)(nil)

// claudeCodeLine is one stdout line (without trailing newline handling; the
// stream loop trims whitespace) paired with the read error, if any, that
// followed it. The reader goroutine closes the channel after sending a line
// with a non-nil error.
type claudeCodeLine struct {
	line []byte
	err  error
}

type claudeCodeProcess struct {
	cmd       *exec.Cmd
	streamCtx context.Context
	lines     chan claudeCodeLine
	pending   *claudeCodeLine
	done      chan error
	stderr    *boundedBuffer

	sawResult bool
	resultErr error

	doneOnce sync.Once
}

func (p *claudeCodeProcess) finish(err error) {
	p.doneOnce.Do(func() {
		p.done <- err
		close(p.done)
	})
}

// boundedBuffer is a concurrency-safe byte buffer that keeps at most max
// bytes, discarding the oldest data on overflow so error messages carry the
// tail of stderr rather than growing without bound.
type boundedBuffer struct {
	mu  sync.Mutex
	max int
	buf bytes.Buffer
}

func newBoundedBuffer(max int) *boundedBuffer {
	return &boundedBuffer{max: max}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := len(p)
	if b.max > 0 {
		if len(p) > b.max {
			p = p[len(p)-b.max:]
		}
		if overflow := b.buf.Len() + len(p) - b.max; overflow > 0 {
			b.buf.Next(overflow)
		}
	}
	_, _ = b.buf.Write(p)
	return n, nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func NewClaudeCodeRunner(cfg ClaudeCodeConfig) *ClaudeCodeRunner {
	if strings.TrimSpace(cfg.BinaryPath) == "" {
		cfg.BinaryPath = defaultClaudeCodeBinaryPath
	}

	return &ClaudeCodeRunner{
		cfg:         cfg,
		initTimeout: defaultClaudeCodeInitTimeout,
		stopGrace:   defaultClaudeCodeStopGrace,
		logger:      log.NewWithOptions(io.Discard, log.Options{}),
		procs:       make(map[int]*claudeCodeProcess),
	}
}

// buildArgs renders the runner config into the headless-mode argument list.
// `-p --output-format stream-json --verbose` is always present; optional
// flags are appended only when configured.
func (r *ClaudeCodeRunner) buildArgs() []string {
	args := []string{"-p", "--output-format", "stream-json", "--verbose"}
	if model := strings.TrimSpace(r.cfg.Model); model != "" {
		args = append(args, "--model", model)
	}
	if mode := strings.TrimSpace(r.cfg.PermissionMode); mode != "" {
		args = append(args, "--permission-mode", mode)
	}
	if r.cfg.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(r.cfg.MaxTurns))
	}
	if len(r.cfg.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(r.cfg.AllowedTools, ","))
	}
	return append(args, r.cfg.ExtraArgs...)
}

func (r *ClaudeCodeRunner) Start(ctx context.Context, _ types.Issue, workspace string, prompt string) (*AgentProcess, error) {
	argv := strings.Fields(strings.TrimSpace(r.cfg.BinaryPath))
	if len(argv) == 0 {
		return nil, errors.New("claude binary path is empty")
	}

	cmd := exec.CommandContext(ctx, argv[0], append(argv[1:], r.buildArgs()...)...)
	cmd.Dir = workspace
	// The prompt is delivered on stdin instead of as a positional argument so
	// large prompts do not hit Windows command-line length limits. exec.Cmd
	// copies the reader into the subprocess and closes stdin at EOF.
	cmd.Stdin = strings.NewReader(prompt)

	stderrBuf := newBoundedBuffer(maxClaudeCodeStderrBytes)
	cmd.Stderr = stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude process: %w", err)
	}

	lines := make(chan claudeCodeLine, 64)
	go readClaudeCodeLines(stdout, lines)

	process := &claudeCodeProcess{
		cmd:       cmd,
		streamCtx: ctx,
		lines:     lines,
		done:      make(chan error, 1),
		stderr:    stderrBuf,
	}

	r.mu.Lock()
	r.procs[cmd.Process.Pid] = process
	r.mu.Unlock()

	sessionID, err := r.awaitFirstLine(ctx, process)
	if err != nil {
		r.cleanupOnStartFailure(process)
		return nil, r.withStderr(err, stderrBuf)
	}

	events := make(chan types.AgentEvent, 512)
	go r.streamEventsAndWait(process, events)

	return &AgentProcess{
		PID:       cmd.Process.Pid,
		SessionID: sessionID,
		Events:    events,
		Done:      process.done,
	}, nil
}

// awaitFirstLine waits for the first stream-json line so the returned
// AgentProcess can carry the claude session_id from the "system"/"init"
// event. The consumed line is stashed on the process and replayed by
// streamEventsAndWait, so no event is lost. With StreamReadTimeout
// configured, a subprocess that never prints anything fails fast; otherwise
// the runner gives up on the session id after a fixed window and keeps
// streaming.
func (r *ClaudeCodeRunner) awaitFirstLine(ctx context.Context, process *claudeCodeProcess) (string, error) {
	// Process startup latency must not trip the stall detector, so the init
	// wait uses the larger of the startup window and the stream timeout.
	initTimeout := r.initTimeout
	if r.cfg.StreamReadTimeout > initTimeout {
		initTimeout = r.cfg.StreamReadTimeout
	}

	timer := time.NewTimer(initTimeout)
	defer timer.Stop()

	select {
	case line, ok := <-process.lines:
		if !ok {
			return "", nil
		}
		process.pending = &line
		return claudeCodeSessionID(line.line), nil
	case <-timer.C:
		if r.cfg.StreamReadTimeout > 0 {
			return "", fmt.Errorf("%w after %s waiting for init event", errClaudeCodeStreamStalled, initTimeout)
		}
		return "", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (r *ClaudeCodeRunner) Stop(proc *AgentProcess) error {
	if proc == nil {
		return errors.New("process is nil")
	}

	r.mu.Lock()
	state, ok := r.procs[proc.PID]
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf("%w: pid %d", errClaudeCodeAlreadyStopped, proc.PID)
	}

	if state.cmd.Process == nil {
		return fmt.Errorf("%w: pid %d", errClaudeCodeAlreadyStopped, proc.PID)
	}

	stopGrace := r.stopGrace
	if err := state.cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		if runtime.GOOS != "windows" {
			return fmt.Errorf("%w: interrupt process: %w", errClaudeCodeStopFailed, err)
		}
		if stopGrace > 100*time.Millisecond {
			stopGrace = 100 * time.Millisecond
		}
	}

	select {
	case doneErr := <-state.done:
		if doneErr != nil && !errors.Is(doneErr, os.ErrProcessDone) {
			r.logger.Warn("claude process exited with error during stop", "pid", proc.PID, "err", doneErr)
		}
		return nil
	case <-time.After(stopGrace):
		if state.cmd.Process == nil {
			return fmt.Errorf("%w: pid %d", errClaudeCodeAlreadyStopped, proc.PID)
		}
		if err := state.cmd.Process.Kill(); err != nil {
			if errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("%w: pid %d", errClaudeCodeAlreadyStopped, proc.PID)
			}
			return fmt.Errorf("%w: kill process: %w", errClaudeCodeStopFailed, err)
		}
		select {
		case doneErr := <-state.done:
			if doneErr != nil && !errors.Is(doneErr, os.ErrProcessDone) {
				r.logger.Warn("claude process exited with error after kill", "pid", proc.PID, "err", doneErr)
			}
			return nil
		case <-time.After(r.stopGrace):
			return fmt.Errorf("%w: timeout after kill", errClaudeCodeStopFailed)
		}
	}
}

// Close is a no-op for ClaudeCodeRunner; each claude process is per-run and
// cleaned up by Stop or by its own exit.
func (r *ClaudeCodeRunner) Close() error { return nil }

// readClaudeCodeLines feeds stdout lines into the channel and closes it after
// forwarding the terminal read error (io.EOF on clean exit).
func readClaudeCodeLines(stdout io.Reader, lines chan<- claudeCodeLine) {
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			lines <- claudeCodeLine{line: line, err: err}
			close(lines)
			return
		}
		lines <- claudeCodeLine{line: line}
	}
}

func (r *ClaudeCodeRunner) streamEventsAndWait(process *claudeCodeProcess, events chan types.AgentEvent) {
	defer close(events)

	var readErr error

stream:
	for {
		var (
			line claudeCodeLine
			ok   bool
		)
		switch {
		case process.pending != nil:
			line, ok = *process.pending, true
			process.pending = nil
		case r.cfg.StreamReadTimeout > 0:
			timer := time.NewTimer(r.cfg.StreamReadTimeout)
			select {
			case line, ok = <-process.lines:
				timer.Stop()
			case <-timer.C:
				readErr = fmt.Errorf("%w after %s: no stdout activity from claude", errClaudeCodeStreamStalled, r.cfg.StreamReadTimeout)
				break stream
			}
		default:
			line, ok = <-process.lines
		}

		if !ok {
			break
		}

		if len(line.line) > maxStreamLineSize {
			preview := line.line
			if len(preview) > 256 {
				preview = preview[:256]
			}
			r.logger.Warn(
				"claude stream line exceeded maximum size",
				"event", "maxStreamLineSize_exceeded",
				"line_bytes", len(line.line),
				"max_bytes", maxStreamLineSize,
				"preview", string(preview),
			)
		} else {
			r.handleStreamLine(process, line.line, events)
		}

		if line.err != nil {
			if !errors.Is(line.err, io.EOF) {
				readErr = line.err
			}
			break
		}
	}

	stalled := errors.Is(readErr, errClaudeCodeStreamStalled)
	if stalled {
		if process.cmd.Process != nil {
			_ = process.cmd.Process.Kill()
		}
		// Drain late lines so the reader goroutine can exit once the
		// pipe closes underneath it.
		go func() {
			for range process.lines {
			}
		}()
	}
	waitErr := process.cmd.Wait()

	ctxErr := process.streamCtx.Err()
	switch {
	case stalled:
		process.finish(r.withStderr(readErr, process.stderr))
	case ctxErr != nil && (waitErr != nil || readErr != nil):
		process.finish(ctxErr)
	case readErr != nil:
		process.finish(r.withStderr(readErr, process.stderr))
	case process.resultErr != nil:
		process.finish(r.withStderr(process.resultErr, process.stderr))
	case waitErr != nil:
		process.finish(r.withStderr(waitErr, process.stderr))
	default:
		process.finish(nil)
	}

	r.mu.Lock()
	delete(r.procs, process.cmd.Process.Pid)
	r.mu.Unlock()
}

// handleStreamLine translates one claude stream-json line into AgentEvents.
// Stream shapes (claude -p --output-format stream-json --verbose):
//
//	{"type":"system","subtype":"init","session_id":...,"model":...}
//	{"type":"assistant","message":{"content":[{"type":"text"|"tool_use",...}],"usage":{...}}}
//	{"type":"user","message":{"content":[{"type":"tool_result",...}]}}
//	{"type":"result","subtype":"success"|"error_max_turns"|"error_during_execution",
//	 "is_error":bool,"result":...,"usage":{"input_tokens":...,"output_tokens":...},
//	 "total_cost_usd":...,"num_turns":...,"session_id":...}
//
// The terminal result is mapped onto the codex taxonomy (turn/completed or
// turn/failed) with usage left at the top level of Data so the orchestrator's
// token accounting picks it up.
func (r *ClaudeCodeRunner) handleStreamLine(process *claudeCodeProcess, raw []byte, events chan<- types.AgentEvent) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return
	}

	msg := map[string]interface{}{}
	if err := json.Unmarshal(trimmed, &msg); err != nil {
		rawPreview := string(trimmed)
		if len(rawPreview) > 512 {
			rawPreview = rawPreview[:512] + "...(truncated)"
		}
		r.logger.Warn("claude malformed JSON", "err", err)
		r.emitEvent(process, events, "protocol/error", map[string]interface{}{
			"error": fmt.Sprintf("malformed JSON: %s", err.Error()),
			"raw":   rawPreview,
		})
		return
	}

	msgType, _ := msg["type"].(string)
	switch msgType {
	case "":
		return
	case "system":
		eventType := "system"
		if subtype, _ := msg["subtype"].(string); subtype != "" {
			eventType = "system/" + subtype
		}
		r.emitEvent(process, events, eventType, msg)
	case "assistant":
		message, _ := msg["message"].(map[string]interface{})
		for _, block := range claudeCodeContentBlocks(message) {
			blockType, _ := block["type"].(string)
			switch blockType {
			case "text":
				r.emitEvent(process, events, "item/agentMessage", map[string]interface{}{
					"text": block["text"],
				})
			case "tool_use":
				r.emitEvent(process, events, "item/toolCall", map[string]interface{}{
					"id":    block["id"],
					"name":  block["name"],
					"input": block["input"],
				})
			}
		}
		if usage, ok := message["usage"].(map[string]interface{}); ok {
			r.emitEvent(process, events, "session.usage", map[string]interface{}{
				"usage": usage,
			})
		}
	case "user":
		message, _ := msg["message"].(map[string]interface{})
		for _, block := range claudeCodeContentBlocks(message) {
			if blockType, _ := block["type"].(string); blockType == "tool_result" {
				r.emitEvent(process, events, "item/toolResult", block)
			}
		}
	case "result":
		process.sawResult = true
		subtype, _ := msg["subtype"].(string)
		isError, _ := msg["is_error"].(bool)
		if isError || (subtype != "" && subtype != "success") {
			reason, _ := msg["result"].(string)
			if strings.TrimSpace(reason) == "" {
				reason = subtype
			}
			if strings.TrimSpace(reason) == "" {
				reason = "unknown failure"
			}
			msg["error"] = reason
			process.resultErr = fmt.Errorf("%w: subtype=%q: %s", errClaudeCodeRunFailed, subtype, reason)
			r.emitEvent(process, events, "turn/failed", msg)
			return
		}
		r.emitEvent(process, events, "turn/completed", msg)
	default:
		r.emitEvent(process, events, msgType, msg)
	}
}

func (r *ClaudeCodeRunner) emitEvent(process *claudeCodeProcess, events chan<- types.AgentEvent, eventType string, data map[string]interface{}) {
	event := types.AgentEvent{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
	}

	streamCtx := process.streamCtx
	if streamCtx == nil {
		streamCtx = context.Background()
	}
	if _, terminal := terminalClaudeCodeEventTypes[event.Type]; terminal {
		select {
		case events <- event:
		case <-streamCtx.Done():
			r.logger.Debug("claude stream context cancelled before terminal event delivery", "event", event.Type, "err", streamCtx.Err())
		}
		return
	}

	select {
	case events <- event:
	default:
	}
}

func (r *ClaudeCodeRunner) cleanupOnStartFailure(process *claudeCodeProcess) {
	if process.cmd.Process != nil {
		_ = process.cmd.Process.Kill()
	}
	// Drain the line channel so the reader goroutine can exit once Wait
	// closes the stdout pipe.
	go func() {
		for range process.lines {
		}
	}()
	_ = process.cmd.Wait()

	r.mu.Lock()
	if process.cmd.Process != nil {
		delete(r.procs, process.cmd.Process.Pid)
	}
	r.mu.Unlock()
}

func (r *ClaudeCodeRunner) withStderr(err error, stderr interface{ String() string }) error {
	if err == nil {
		return nil
	}
	if stderr == nil {
		return err
	}
	text := strings.TrimSpace(stderr.String())
	if text == "" {
		return err
	}
	return fmt.Errorf("%w: stderr: %s", err, text)
}

func claudeCodeContentBlocks(message map[string]interface{}) []map[string]interface{} {
	raw, ok := message["content"].([]interface{})
	if !ok {
		return nil
	}
	blocks := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		if block, ok := item.(map[string]interface{}); ok {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// claudeCodeSessionID extracts the session_id carried by every claude
// stream-json message (the "system"/"init" event in the common case).
func claudeCodeSessionID(line []byte) string {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return ""
	}
	msg := map[string]interface{}{}
	if err := json.Unmarshal(line, &msg); err != nil {
		return ""
	}
	sessionID, _ := msg["session_id"].(string)
	return strings.TrimSpace(sessionID)
}
