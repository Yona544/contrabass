package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/orchestrator"
	"github.com/junhoyeo/contrabass/internal/types"
)

func newAuthTestHandler(t *testing.T, token string) http.Handler {
	t.Helper()

	provider := fakeSnapshotProvider{snapshot: orchestrator.StateSnapshot{Issues: map[string]types.Issue{}}}
	s := &Server{snapshotProvider: provider, dashboardFS: nil}
	s.SetAuthToken(token)
	return s.withAuth(s.newMux())
}

func TestAuthDisabledLeavesRoutesOpen(t *testing.T) {
	h := newAuthTestHandler(t, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/state", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthRejectsMissingAndWrongCredentials(t *testing.T) {
	h := newAuthTestHandler(t, "secret-token")

	tests := []struct {
		name  string
		setup func(*http.Request)
	}{
		{name: "no_credentials", setup: func(*http.Request) {}},
		{name: "wrong_bearer", setup: func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer nope")
		}},
		{name: "wrong_cookie", setup: func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: authCookieName, Value: "nope"})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/state", nil)
			tt.setup(req)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.Contains(t, rec.Body.String(), "auth token")
		})
	}
}

func TestAuthAcceptsBearerAndCookie(t *testing.T) {
	h := newAuthTestHandler(t, "secret-token")

	bearer := httptest.NewRequest(http.MethodGet, "/api/v1/state", nil)
	bearer.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, bearer)
	assert.Equal(t, http.StatusOK, rec.Code)

	cookie := httptest.NewRequest(http.MethodGet, "/api/v1/state", nil)
	cookie.AddCookie(&http.Cookie{Name: authCookieName, Value: "secret-token"})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, cookie)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthTokenQueryExchangesForCookie(t *testing.T) {
	h := newAuthTestHandler(t, "secret-token")

	req := httptest.NewRequest(http.MethodGet, "/?token=secret-token", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/", rec.Header().Get("Location"))

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, authCookieName, cookies[0].Name)
	assert.Equal(t, "secret-token", cookies[0].Value)
	assert.True(t, cookies[0].HttpOnly)
}

func TestAuthTokenQueryRejectsWrongToken(t *testing.T) {
	h := newAuthTestHandler(t, "secret-token")

	req := httptest.NewRequest(http.MethodGet, "/?token=wrong", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, rec.Result().Cookies())
}

func TestAuthAllowsPreflightWithoutCredentials(t *testing.T) {
	h := newAuthTestHandler(t, "secret-token")

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/refresh", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestIsLoopbackListenAddr(t *testing.T) {
	loopback := []string{"localhost:8080", "LOCALHOST:9090", "127.0.0.1:8080", "[::1]:8080"}
	for _, addr := range loopback {
		assert.True(t, IsLoopbackListenAddr(addr), "expected %q to be loopback", addr)
	}

	open := []string{"0.0.0.0:8080", ":8080", "192.168.1.10:8080", "example.com:8080", "100.101.102.103:8080", ""}
	for _, addr := range open {
		assert.False(t, IsLoopbackListenAddr(addr), "expected %q to be non-loopback", addr)
	}
}
