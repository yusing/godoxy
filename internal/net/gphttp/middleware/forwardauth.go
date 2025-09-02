package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/route/routes"
)

type (
	forwardAuthMiddleware struct {
		ForwardAuthMiddlewareOpts
	}

	ForwardAuthMiddlewareOpts struct {
		ForwardAuthRoute    string   `json:"forwardauth_route"`    // default: "tinyauth"
		ForwardAuthLogin    string   `json:"forwardauth_login"`    // the redirect login path, e.g. "/login?redirect_uri="
		ForwardAuthEndpoint string   `json:"forwardauth_endpoint"` // default: "/api/auth/nginx"
		ForwardAuthHeaders  []string `json:"forwardauth_headers"`  // additional headers to forward from auth server to upstream, e.g. ["Remote-User", "Remote-Name"]
	}
)

var ForwardAuth = NewMiddleware[forwardAuthMiddleware]()

func (m *forwardAuthMiddleware) setup() {
	m.ForwardAuthMiddlewareOpts = ForwardAuthMiddlewareOpts{
		ForwardAuthRoute:    "tinyauth",
		ForwardAuthLogin:    "/login?redirect_uri=",
		ForwardAuthEndpoint: "/api/auth/nginx",
		ForwardAuthHeaders:  []string{"Remote-User", "Remote-Name", "Remote-Email", "Remote-Groups"},
	}
}

// before implements RequestModifier.
func (m *forwardAuthMiddleware) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	route, ok := routes.HTTP.Get(m.ForwardAuthRoute)
	if !ok {
		log.Warn().Str("route", m.ForwardAuthRoute).Msg("forwardauth route not found")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}

	forwardAuthUrl := *route.TargetURL()
	forwardAuthUrl.Path = m.ForwardAuthEndpoint

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, forwardAuthUrl.String(), nil)
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

	if resp.StatusCode == http.StatusOK {
		for _, h := range m.ForwardAuthHeaders {
			if v := resp.Header.Get(h); v != "" {
				w.Header().Set(h, v)
			}
		}
		return true
	}

	if resp.StatusCode == http.StatusUnauthorized {
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

	if resp.StatusCode == http.StatusForbidden {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}

	return false
}
