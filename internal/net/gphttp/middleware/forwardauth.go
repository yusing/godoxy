package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type (
	forwardAuthMiddleware struct {
		initMu sync.Mutex
		ForwardAuthMiddlewareOpts
	}

	ForwardAuthMiddlewareOpts struct {
		ForwardAuthRoute    string `json:"forwardauth_route"`    // default: "tinyauth"
		ForwardAuthPort     int    `json:"forwardauth_port"`     // default: 3000
		ForwardAuthLogin    string `json:"forwardauth_login"`    // the redirect login path, e.g. "/login?redirect_uri="
		ForwardAuthEndpoint string `json:"forwardauth_endpoint"` // default: "/api/auth/nginx"
	}
)

var ForwardAuth = NewMiddleware[forwardAuthMiddleware]()

func (m *forwardAuthMiddleware) setup() {
	m.ForwardAuthMiddlewareOpts = ForwardAuthMiddlewareOpts{
		ForwardAuthRoute:    "tinyauth",
		ForwardAuthPort:     3000,
		ForwardAuthLogin:    "/login?redirect_uri=",
		ForwardAuthEndpoint: "/api/auth/nginx",
	}
}

// before implements RequestModifier.
func (m *forwardAuthMiddleware) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	m.initMu.Lock()
	defer m.initMu.Unlock()

	forwardAuthUrl := fmt.Sprintf("http://localhost:%d%s", m.ForwardAuthPort, m.ForwardAuthEndpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, forwardAuthUrl, nil)
	if err != nil {
		log.Err(err).Msg("failed to create request")
		return false
	}

	req.Header = r.Header.Clone()
	req.Header.Set("X-Forwarded-Proto", r.Proto)
	req.Header.Set("X-Forwarded-Host", r.Host)
	req.Header.Set("X-Forwarded-Uri", r.URL.RequestURI())

	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		log.Err(err).Msg("failed to connect to forwardauth server")
		return false
	}

	defer resp.Body.Close()
	status_code := resp.StatusCode

	if status_code == 200 {
		// copy Remote- headers from resp to req
		// e.g. Remote-User, Remote-Email, Remote-Name, Remote-Groups
		// but it do nothing if tinyauthMiddleware is after ModifyResponse middleware
		for k, values := range resp.Header {
			key := strings.ToLower(k)
			if strings.HasPrefix(key, "remote-") {
				for _, v := range values {
					r.Header[k] = append(r.Header[k], v)
				}
			}
		}
		return true
	}

	if status_code == 401 {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}

		scheme := "http://"

		if r.TLS != nil {
			scheme = "https://"
		}

		parts := strings.Split(host, ".")
		if len(parts) > 2 {
			host = strings.Join(parts[len(parts)-2:], ".")

		}
		redirectUrl := scheme + m.ForwardAuthRoute + "." + host + m.ForwardAuthLogin + scheme + r.Host + r.URL.RequestURI()
		http.Redirect(w, r, redirectUrl, http.StatusPermanentRedirect)
		return false
	}

	if status_code == 403 {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}

	return false
}
