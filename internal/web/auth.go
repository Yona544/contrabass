package web

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

// authCookieName carries the dashboard token after the one-time ?token=
// exchange so the SPA and its EventSource connections (which cannot set an
// Authorization header) authenticate via same-origin cookies.
const authCookieName = "contrabass_token"

// SetAuthToken enables bearer-token authentication for every route. An empty
// token leaves the server open, which is only acceptable for loopback binds —
// callers enforce that via IsLoopbackListenAddr before starting the server.
func (s *Server) SetAuthToken(token string) {
	s.authToken = strings.TrimSpace(token)
}

// ListenAddr returns the normalized address the server binds.
func (s *Server) ListenAddr() string {
	return s.listenAddr
}

// IsLoopbackListenAddr reports whether addr only accepts connections from the
// local machine. Addresses without a resolvable loopback host (including
// ":port" all-interface binds and hostnames other than localhost) count as
// non-loopback so callers fail safe.
func IsLoopbackListenAddr(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		host = strings.TrimSpace(addr)
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// withAuth gates every route behind the configured token. Browsers sign in
// once via /?token=<token>, which sets an HttpOnly cookie and redirects to the
// same URL without the query parameter; API clients send
// "Authorization: Bearer <token>". OPTIONS preflight requests pass through
// because they carry no credentials by specification.
func (s *Server) withAuth(next http.Handler) http.Handler {
	if s.authToken == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		if token := r.URL.Query().Get("token"); token != "" {
			if s.tokenMatches(token) {
				http.SetCookie(w, &http.Cookie{
					Name:     authCookieName,
					Value:    token,
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
				query := r.URL.Query()
				query.Del("token")
				redirect := *r.URL
				redirect.RawQuery = query.Encode()
				http.Redirect(w, r, redirect.String(), http.StatusSeeOther)
				return
			}
			s.rejectUnauthorized(w, r)
			return
		}

		if s.authorized(r) {
			next.ServeHTTP(w, r)
			return
		}
		s.rejectUnauthorized(w, r)
	})
}

func (s *Server) authorized(r *http.Request) bool {
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		if s.tokenMatches(strings.TrimPrefix(header, "Bearer ")) {
			return true
		}
	}
	if cookie, err := r.Cookie(authCookieName); err == nil && s.tokenMatches(cookie.Value) {
		return true
	}
	return false
}

func (s *Server) tokenMatches(candidate string) bool {
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(s.authToken)) == 1
}

func (s *Server) rejectUnauthorized(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSONError(w, http.StatusUnauthorized, "missing or invalid auth token")
		return
	}
	http.Error(w, "Unauthorized: open /?token=<dashboard token> to sign in.", http.StatusUnauthorized)
}
