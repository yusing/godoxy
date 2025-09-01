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

type tinyAuthMiddleware struct {
	// tinyauthRoute string
	TinyauthRoute string
	TinyauthPort  int
	initMu        sync.Mutex
}

var TinyAuth = NewMiddleware[tinyAuthMiddleware]()

// before implements RequestModifier.
func (m *tinyAuthMiddleware) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	m.initMu.Lock()
	defer m.initMu.Unlock()
	endpoint := "/api/auth/nginx"

	if m.TinyauthRoute == "" {
		m.TinyauthRoute = "tinyauth"
	}
	if m.TinyauthPort == 0 {
		m.TinyauthPort = 3000
	}

	tinyAuthUrl := fmt.Sprintf("http://localhost:%d%s", m.TinyauthPort, endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tinyAuthUrl, nil)
	req.Header = r.Header.Clone()
	req.Header.Set("X-Forwarded-Proto", r.Proto)
	req.Header.Set("X-Forwarded-Host", r.Host)
	req.Header.Set("X-Forwarded-Uri", r.URL.RequestURI())
	if err != nil {
		log.Err(err).Msg("failed to create request")
		return false
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		log.Err(err).Msg("failed to connect to tinyauth")
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
					r.Header[key] = append(r.Header[key], v)
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

		parts := strings.Split(host, ".")
		if len(parts) > 2 {
			host = strings.Join(parts[len(parts)-2:], ".")

		}
		redirectUrl := r.Host + r.URL.RequestURI()
		if r.TLS != nil {
			redirectUrl = fmt.Sprintf("https://%s.%s/login?redirect_uri=https://%s", m.TinyauthRoute, host, redirectUrl)
		} else {
			redirectUrl = fmt.Sprintf("http://%s.%s/login?redirect_uri=http://%s", m.TinyauthRoute, host, redirectUrl)
		}

		http.Redirect(w, r, redirectUrl, http.StatusPermanentRedirect)
		return false
	}

	if status_code == 403 {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}

	return false
}
