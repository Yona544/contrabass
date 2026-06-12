// Package notify posts orchestrator and team lifecycle events to chat
// webhooks (Slack-compatible and generic JSON) so the team learns about
// finished, failed, or retried runs without watching a terminal.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/log"

	"github.com/junhoyeo/contrabass/internal/hub"
	"github.com/junhoyeo/contrabass/internal/orchestrator"
	"github.com/junhoyeo/contrabass/internal/types"
	"github.com/junhoyeo/contrabass/internal/web"
)

const (
	defaultQueueSize   = 64
	defaultPostTimeout = 10 * time.Second
)

// defaultEvents is the filter applied when the workflow does not configure
// notifications.events: completion, failure, and retry signals — the moments
// a teammate would want to be interrupted for.
var defaultEvents = []string{
	"AgentFinished",
	"BackoffEnqueued",
	"ScheduleWindowClosed",
	"run_completed",
	"run_error",
}

type Config struct {
	// SlackWebhookURL receives {"text": "..."} payloads (Slack incoming
	// webhook format, also accepted by Discord's /slack endpoint).
	SlackWebhookURL string
	// WebhookURL receives the structured JSON envelope for custom sinks.
	WebhookURL string
	// Events filters by web.WebEvent Type (e.g. "AgentFinished",
	// "run_error"). Empty means defaultEvents; ["*"] forwards everything.
	Events     []string
	HTTPClient *http.Client
	Logger     *log.Logger
}

type Notifier struct {
	slackURL   string
	webhookURL string
	events     map[string]struct{}
	all        bool
	client     *http.Client
	logger     *log.Logger
	queue      chan web.WebEvent
}

// envelope is the generic-webhook JSON body.
type envelope struct {
	Source    string    `json:"source"`
	Kind      string    `json:"kind"`
	Type      string    `json:"type"`
	IssueID   string    `json:"issue_id,omitempty"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

func New(cfg Config) *Notifier {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultPostTimeout}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	selected := cfg.Events
	if len(selected) == 0 {
		selected = defaultEvents
	}
	events := make(map[string]struct{}, len(selected))
	all := false
	for _, name := range selected {
		name = strings.TrimSpace(name)
		if name == "*" {
			all = true
			continue
		}
		if name != "" {
			events[name] = struct{}{}
		}
	}

	return &Notifier{
		slackURL:   strings.TrimSpace(cfg.SlackWebhookURL),
		webhookURL: strings.TrimSpace(cfg.WebhookURL),
		events:     events,
		all:        all,
		client:     client,
		logger:     logger,
		queue:      make(chan web.WebEvent, defaultQueueSize),
	}
}

// Enabled reports whether any sink is configured.
func (n *Notifier) Enabled() bool {
	return n != nil && (n.slackURL != "" || n.webhookURL != "")
}

// Start drains the queue until ctx is done. Posting happens here so Notify
// never blocks an event-emitting hot path.
func (n *Notifier) Start(ctx context.Context) {
	if !n.Enabled() {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-n.queue:
			n.post(ctx, evt)
		}
	}
}

// RunHub subscribes to the web event hub and enqueues matching events. It
// returns when ctx is done or the hub closes the subscription.
func (n *Notifier) RunHub(ctx context.Context, h *hub.Hub[web.WebEvent]) {
	if !n.Enabled() || h == nil {
		return
	}
	id, events := h.Subscribe()
	defer h.Unsubscribe(id)
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			n.Notify(evt)
		}
	}
}

// Notify enqueues an event without blocking; when the queue is full the event
// is dropped with a warning rather than stalling the orchestrator.
func (n *Notifier) Notify(evt web.WebEvent) {
	if !n.Enabled() || !n.matches(evt.Type) {
		return
	}
	select {
	case n.queue <- evt:
	default:
		n.logger.Warn("notification queue full, dropping event", "type", evt.Type)
	}
}

func (n *Notifier) matches(eventType string) bool {
	if n.all {
		return true
	}
	_, ok := n.events[eventType]
	return ok
}

func (n *Notifier) post(ctx context.Context, evt web.WebEvent) {
	message, issueID := renderMessage(evt)

	if n.slackURL != "" {
		body := map[string]string{"text": message}
		if err := n.postJSON(ctx, n.slackURL, body); err != nil {
			n.logger.Warn("slack notification failed", "type", evt.Type, "err", err)
		}
	}
	if n.webhookURL != "" {
		body := envelope{
			Source:    "contrabass",
			Kind:      string(evt.Kind),
			Type:      evt.Type,
			IssueID:   issueID,
			Message:   message,
			Timestamp: evt.Timestamp,
		}
		if err := n.postJSON(ctx, n.webhookURL, body); err != nil {
			n.logger.Warn("webhook notification failed", "type", evt.Type, "err", err)
		}
	}
}

func (n *Notifier) postJSON(ctx context.Context, url string, body interface{}) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, defaultPostTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("post notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notification endpoint returned status %d", resp.StatusCode)
	}
	return nil
}

// renderMessage produces the human-readable chat line for an event and the
// issue identifier when one is known.
func renderMessage(evt web.WebEvent) (string, string) {
	switch payload := evt.Payload.(type) {
	case orchestrator.OrchestratorEvent:
		return renderOrchestratorMessage(payload), payload.IssueID
	case types.TeamEvent:
		return renderTeamMessage(payload), ""
	case web.ScheduleEvent:
		return "🗓️ " + payload.Summary, ""
	default:
		return fmt.Sprintf("contrabass event %s", evt.Type), ""
	}
}

func renderOrchestratorMessage(evt orchestrator.OrchestratorEvent) string {
	switch data := evt.Data.(type) {
	case orchestrator.AgentFinished:
		if data.Error != "" {
			return fmt.Sprintf("❌ %s failed (attempt %d): %s", evt.IssueID, data.Attempt, data.Error)
		}
		return fmt.Sprintf("✅ %s finished (attempt %d, phase %s, tokens %d in / %d out)",
			evt.IssueID, data.Attempt, data.Phase, data.TokensIn, data.TokensOut)
	case orchestrator.AgentStarted:
		return fmt.Sprintf("▶️ %s started (attempt %d, workspace %s)", evt.IssueID, data.Attempt, data.Workspace)
	case orchestrator.BackoffEnqueued:
		return fmt.Sprintf("🔁 %s retry queued (attempt %d, retry at %s): %s",
			evt.IssueID, data.Attempt, data.RetryAt.Format(time.RFC3339), data.Error)
	case orchestrator.IssueReleased:
		return fmt.Sprintf("🏁 %s released (attempt %d)", evt.IssueID, data.Attempt)
	case orchestrator.ScheduleWindowClosed:
		return "🗓️ " + data.Summary
	default:
		return fmt.Sprintf("contrabass %s for %s", evt.Type.String(), evt.IssueID)
	}
}

func renderTeamMessage(evt types.TeamEvent) string {
	detail := ""
	if len(evt.Data) > 0 {
		if encoded, err := json.Marshal(evt.Data); err == nil {
			detail = " " + string(encoded)
		}
	}
	return fmt.Sprintf("👥 team %s: %s%s", evt.TeamName, evt.Type, detail)
}
