package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/history"
	"github.com/junhoyeo/contrabass/internal/orchestrator"
	"github.com/junhoyeo/contrabass/internal/types"
)

type fakeController struct {
	paused   bool
	retryIDs []string
	retryOK  bool
}

func (f *fakeController) SetDispatchPaused(paused bool) { f.paused = paused }
func (f *fakeController) DispatchPaused() bool          { return f.paused }
func (f *fakeController) RetryNow(issueID string) bool {
	f.retryIDs = append(f.retryIDs, issueID)
	return f.retryOK
}

func newControlTestServer(controller DispatchController, provider HistoryProvider) http.Handler {
	s := &Server{
		snapshotProvider: fakeSnapshotProvider{snapshot: orchestrator.StateSnapshot{Issues: map[string]types.Issue{}}},
	}
	if controller != nil {
		s.SetDispatchController(controller)
	}
	if provider != nil {
		s.SetHistoryProvider(provider)
	}
	return s.newMux()
}

func TestControlEndpointsUnavailableWithoutController(t *testing.T) {
	h := newControlTestServer(nil, nil)

	for _, target := range []string{"/api/v1/control/pause", "/api/v1/control/resume", "/api/v1/backoff/CB-1/retry"} {
		req := httptest.NewRequest(http.MethodPost, target, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code, target)
	}
}

func TestPauseAndResumeDispatch(t *testing.T) {
	controller := &fakeController{}
	h := newControlTestServer(controller, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/control/pause", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, controller.paused)
	assert.JSONEq(t, `{"dispatch_paused": true}`, rec.Body.String())

	req = httptest.NewRequest(http.MethodPost, "/api/v1/control/resume", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, controller.paused)
}

func TestRetryNowEndpoint(t *testing.T) {
	controller := &fakeController{retryOK: true}
	h := newControlTestServer(controller, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backoff/CB-7/retry", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, []string{"CB-7"}, controller.retryIDs)

	controller.retryOK = false
	req = httptest.NewRequest(http.MethodPost, "/api/v1/backoff/CB-8/retry", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHistoryAndAnalyticsEndpoints(t *testing.T) {
	store := history.NewStore(t.TempDir())
	require.NoError(t, store.Append(history.Record{
		IssueID:    "CB-1",
		AgentType:  "claude",
		Attempt:    1,
		Phase:      "Succeeded",
		Succeeded:  true,
		TokensIn:   10,
		TokensOut:  20,
		FinishedAt: time.Now().UTC(),
	}))

	h := newControlTestServer(nil, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/history?limit=10", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var payload struct {
		Records []history.Record `json:"records"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Records, 1)
	assert.Equal(t, "CB-1", payload.Records[0].IssueID)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/analytics", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var analytics history.Analytics
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &analytics))
	assert.Equal(t, 1, analytics.TotalRuns)
	assert.Equal(t, 1, analytics.ByAgent["claude"].Runs)
}

func TestHistoryRejectsBadLimit(t *testing.T) {
	h := newControlTestServer(nil, history.NewStore(t.TempDir()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/history?limit=nope", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHistoryUnavailableWithoutProvider(t *testing.T) {
	h := newControlTestServer(nil, nil)

	for _, target := range []string{"/api/v1/history", "/api/v1/analytics"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code, target)
	}
}
