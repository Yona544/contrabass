package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/hub"
	"github.com/junhoyeo/contrabass/internal/orchestrator"
	"github.com/junhoyeo/contrabass/internal/types"
	"github.com/junhoyeo/contrabass/internal/web"
)

type capturedRequest struct {
	body []byte
}

func newCaptureServer(t *testing.T) (*httptest.Server, func() []capturedRequest) {
	t.Helper()

	var mu sync.Mutex
	var captured []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		mu.Lock()
		captured = append(captured, capturedRequest{body: body})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	return srv, func() []capturedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]capturedRequest(nil), captured...)
	}
}

func finishedEvent(issueID string, errMsg string) web.WebEvent {
	return web.NewOrchestratorWebEvent(orchestrator.OrchestratorEvent{
		Type:    orchestrator.EventAgentFinished,
		IssueID: issueID,
		Data: orchestrator.AgentFinished{
			Attempt:   2,
			TokensIn:  100,
			TokensOut: 50,
			Error:     errMsg,
		},
		Timestamp: time.Now().UTC(),
	})
}

func drainNotifier(t *testing.T, n *Notifier, requests func() []capturedRequest, want int) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go n.Start(ctx)

	require.Eventually(t, func() bool {
		return len(requests()) >= want
	}, 5*time.Second, 10*time.Millisecond)
}

func TestNotifierDisabledWithoutSinks(t *testing.T) {
	n := New(Config{})
	assert.False(t, n.Enabled())
	// Notify must be a no-op rather than a panic or block.
	n.Notify(finishedEvent("CB-1", ""))
}

func TestNotifierPostsSlackText(t *testing.T) {
	srv, requests := newCaptureServer(t)
	n := New(Config{SlackWebhookURL: srv.URL})

	n.Notify(finishedEvent("CB-7", ""))
	drainNotifier(t, n, requests, 1)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(requests()[0].body, &payload))
	assert.Contains(t, payload["text"], "✅ CB-7 finished")
	assert.Contains(t, payload["text"], "100 in / 50 out")
}

func TestNotifierPostsGenericEnvelope(t *testing.T) {
	srv, requests := newCaptureServer(t)
	n := New(Config{WebhookURL: srv.URL})

	n.Notify(finishedEvent("CB-9", "boom"))
	drainNotifier(t, n, requests, 1)

	var payload envelope
	require.NoError(t, json.Unmarshal(requests()[0].body, &payload))
	assert.Equal(t, "contrabass", payload.Source)
	assert.Equal(t, "orchestrator", payload.Kind)
	assert.Equal(t, "AgentFinished", payload.Type)
	assert.Equal(t, "CB-9", payload.IssueID)
	assert.Contains(t, payload.Message, "❌ CB-9 failed")
	assert.Contains(t, payload.Message, "boom")
}

func TestNotifierDefaultFilterSkipsStatusUpdates(t *testing.T) {
	srv, requests := newCaptureServer(t)
	n := New(Config{SlackWebhookURL: srv.URL})

	n.Notify(web.NewOrchestratorWebEvent(orchestrator.OrchestratorEvent{
		Type:      orchestrator.EventStatusUpdate,
		Data:      orchestrator.StatusUpdate{},
		Timestamp: time.Now().UTC(),
	}))
	n.Notify(finishedEvent("CB-2", ""))
	drainNotifier(t, n, requests, 1)

	// Give the worker a beat to (incorrectly) post the StatusUpdate too.
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, requests(), 1)
}

func TestNotifierWildcardForwardsEverything(t *testing.T) {
	srv, requests := newCaptureServer(t)
	n := New(Config{SlackWebhookURL: srv.URL, Events: []string{"*"}})

	n.Notify(web.NewOrchestratorWebEvent(orchestrator.OrchestratorEvent{
		Type:      orchestrator.EventStatusUpdate,
		Data:      orchestrator.StatusUpdate{},
		Timestamp: time.Now().UTC(),
	}))
	drainNotifier(t, n, requests, 1)
}

func TestNotifierRendersTeamEvents(t *testing.T) {
	srv, requests := newCaptureServer(t)
	n := New(Config{SlackWebhookURL: srv.URL, Events: []string{"task_completed"}})

	n.Notify(web.NewTeamWebEvent(types.TeamEvent{
		Type:      "task_completed",
		TeamName:  "issue-cb-1",
		Data:      map[string]interface{}{"task_id": "001"},
		Timestamp: time.Now().UTC(),
	}))
	drainNotifier(t, n, requests, 1)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(requests()[0].body, &payload))
	assert.Contains(t, payload["text"], "👥 team issue-cb-1: task_completed")
	assert.Contains(t, payload["text"], "task_id")
}

func TestNotifierRunHubConsumesSubscription(t *testing.T) {
	srv, requests := newCaptureServer(t)
	n := New(Config{SlackWebhookURL: srv.URL})

	source := make(chan web.WebEvent, 4)
	h := hub.NewHub(source)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.Run(ctx)
	go n.Start(ctx)
	go n.RunHub(ctx, h)

	require.Eventually(t, func() bool {
		return h.SubscriberCount() == 1
	}, 5*time.Second, 10*time.Millisecond)

	source <- finishedEvent("CB-3", "")
	require.Eventually(t, func() bool {
		return len(requests()) == 1
	}, 5*time.Second, 10*time.Millisecond)
}

func TestNotifierSurvivesFailingEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	good, requests := newCaptureServer(t)
	n := New(Config{SlackWebhookURL: srv.URL, WebhookURL: good.URL})

	n.Notify(finishedEvent("CB-4", ""))
	drainNotifier(t, n, requests, 1)
}
