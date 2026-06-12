package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/junhoyeo/contrabass/internal/config"
	"github.com/junhoyeo/contrabass/internal/web"
)

// webOptions resolves where (and whether) the dashboard listens and which
// token gates it. Precedence: --listen flag > web.listen config >
// localhost:<--port>; the dashboard stays disabled when none are set.
type webOptions struct {
	Enabled    bool
	ListenAddr string
	AuthToken  string
}

func resolveWebOptions(cfg *config.WorkflowConfig, flagListen string, port int) (webOptions, error) {
	listen := strings.TrimSpace(flagListen)
	if listen == "" {
		listen = cfg.WebListen()
	}
	if listen == "" {
		if port <= 0 {
			return webOptions{}, nil
		}
		listen = fmt.Sprintf("localhost:%d", port)
	}

	if strings.HasPrefix(listen, ":") {
		return webOptions{}, fmt.Errorf(
			"ambiguous listen address %q: use localhost:%s for local-only or 0.0.0.0:%s to accept remote connections",
			listen, listen[1:], listen[1:],
		)
	}

	token := webAuthToken(cfg)
	if !web.IsLoopbackListenAddr(listen) && token == "" {
		return webOptions{}, fmt.Errorf(
			"refusing to serve the dashboard on non-loopback address %q without a token: set web.auth_token or CONTRABASS_DASHBOARD_TOKEN",
			listen,
		)
	}

	return webOptions{Enabled: true, ListenAddr: listen, AuthToken: token}, nil
}

// webAuthToken prefers the environment over the workflow file so tokens stay
// out of committed configs, mirroring how tracker credentials resolve.
func webAuthToken(cfg *config.WorkflowConfig) string {
	if env := strings.TrimSpace(os.Getenv("CONTRABASS_DASHBOARD_TOKEN")); env != "" {
		return env
	}
	return cfg.WebAuthToken()
}

// dashboardURL renders the address users should open, including the one-time
// token sign-in query when auth is enabled.
func (o webOptions) dashboardURL() string {
	if !o.Enabled {
		return ""
	}
	if o.AuthToken == "" {
		return fmt.Sprintf("http://%s", o.ListenAddr)
	}
	return fmt.Sprintf("http://%s/?token=%s", o.ListenAddr, o.AuthToken)
}
