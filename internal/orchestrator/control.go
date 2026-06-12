package orchestrator

import (
	"time"

	"github.com/junhoyeo/contrabass/internal/history"
	"github.com/junhoyeo/contrabass/internal/logging"
	"github.com/junhoyeo/contrabass/internal/types"
)

// SetDispatchPaused globally pauses or resumes new agent dispatches. Running
// agents are unaffected; the dashboard exposes this as the pause button.
func (o *Orchestrator) SetDispatchPaused(paused bool) {
	o.dispatchPaused.Store(paused)
	if paused {
		logging.LogOrchestratorEvent(o.logger, "dispatch_paused")
	} else {
		logging.LogOrchestratorEvent(o.logger, "dispatch_resumed")
	}
	o.emitStatusUpdate()
}

// DispatchPaused reports whether new dispatches are globally paused.
func (o *Orchestrator) DispatchPaused() bool {
	return o.dispatchPaused.Load()
}

// RetryNow promotes a backoff entry so the next poll cycle retries it
// immediately. Returns false when the issue is not waiting in backoff.
func (o *Orchestrator) RetryNow(issueID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i := range o.backoff {
		if o.backoff[i].IssueID == issueID {
			o.backoff[i].RetryAt = time.Now()
			logging.LogIssueEvent(o.logger, issueID, "retry_now_requested")
			return true
		}
	}
	return false
}

// SetRunHistory installs the persistent run log; nil disables recording.
func (o *Orchestrator) SetRunHistory(store *history.Store) {
	o.history = store
}

// recordRunHistory appends a finished attempt to the run log (best-effort).
func (o *Orchestrator) recordRunHistory(issue types.Issue, attempt types.RunAttempt) {
	if o.history == nil {
		return
	}

	finished := time.Now()
	durationMs := int64(0)
	if !attempt.StartTime.IsZero() {
		durationMs = finished.Sub(attempt.StartTime).Milliseconds()
	}

	rec := history.Record{
		IssueID:    issue.ID,
		Identifier: issue.Identifier,
		Title:      issue.Title,
		AgentType:  o.currentConfig().AgentType(),
		Model:      o.effectiveModel(),
		SessionID:  attempt.SessionID,
		Attempt:    attempt.Attempt,
		Phase:      attempt.Phase.String(),
		Succeeded:  attempt.Phase == types.Succeeded,
		Error:      attempt.Error,
		TokensIn:   attempt.TokensIn,
		TokensOut:  attempt.TokensOut,
		StartedAt:  attempt.StartTime,
		FinishedAt: finished,
		DurationMs: durationMs,
	}
	if err := o.history.Append(rec); err != nil {
		logging.LogIssueEvent(o.logger, issue.ID, "history_append_failed", "err", err)
	}
}

// effectiveModel resolves the model the configured agent actually runs with,
// preferring agent-specific overrides over the top-level model field.
func (o *Orchestrator) effectiveModel() string {
	cfg := o.currentConfig()
	switch cfg.AgentType() {
	case "claude":
		return cfg.ClaudeModel()
	case "codex":
		return cfg.CodexModel()
	default:
		model, _ := cfg.Model()
		return model
	}
}

// runCostUSD estimates the dollar cost of an attempt via the history store's
// pricing table; 0 when history is disabled or the model is unpriced.
func (o *Orchestrator) runCostUSD(attempt types.RunAttempt) float64 {
	if o.history == nil {
		return 0
	}
	return o.history.Cost(o.effectiveModel(), attempt.TokensIn, attempt.TokensOut)
}
