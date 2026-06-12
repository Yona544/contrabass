package web

import (
	"net/http"
	"strings"

	"github.com/junhoyeo/contrabass/internal/history"
)

// DispatchController is implemented by the orchestrator so the dashboard can
// pause/resume dispatch and promote backoff retries.
type DispatchController interface {
	SetDispatchPaused(paused bool)
	DispatchPaused() bool
	RetryNow(issueID string) bool
}

// HistoryProvider exposes the persistent run log to the dashboard.
type HistoryProvider interface {
	Records(limit int) ([]history.Record, error)
	Analytics() (history.Analytics, error)
}

func (s *Server) SetDispatchController(controller DispatchController) {
	s.dispatchController = controller
}

func (s *Server) SetHistoryProvider(provider HistoryProvider) {
	s.historyProvider = provider
}

func (s *Server) handlePauseDispatch(w http.ResponseWriter, _ *http.Request) {
	if s.dispatchController == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "control plane not configured")
		return
	}
	s.dispatchController.SetDispatchPaused(true)
	writeJSON(w, http.StatusOK, map[string]bool{"dispatch_paused": true})
}

func (s *Server) handleResumeDispatch(w http.ResponseWriter, _ *http.Request) {
	if s.dispatchController == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "control plane not configured")
		return
	}
	s.dispatchController.SetDispatchPaused(false)
	writeJSON(w, http.StatusOK, map[string]bool{"dispatch_paused": false})
}

func (s *Server) handleRetryNow(w http.ResponseWriter, r *http.Request) {
	if s.dispatchController == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "control plane not configured")
		return
	}
	issueID := strings.TrimSpace(r.PathValue("issue_id"))
	if issueID == "" {
		writeJSONError(w, http.StatusBadRequest, "issue_id is required")
		return
	}
	if !s.dispatchController.RetryNow(issueID) {
		writeJSONError(w, http.StatusNotFound, "issue is not waiting in the retry queue")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if s.historyProvider == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "run history not configured")
		return
	}

	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed := 0
		for _, c := range raw {
			if c < '0' || c > '9' {
				parsed = -1
				break
			}
			parsed = parsed*10 + int(c-'0')
		}
		if parsed < 0 {
			writeJSONError(w, http.StatusBadRequest, "limit must be a non-negative integer")
			return
		}
		limit = parsed
	}

	records, err := s.historyProvider.Records(limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if records == nil {
		records = []history.Record{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"records": records})
}

func (s *Server) handleAnalytics(w http.ResponseWriter, _ *http.Request) {
	if s.historyProvider == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "run history not configured")
		return
	}

	analytics, err := s.historyProvider.Analytics()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, analytics)
}
