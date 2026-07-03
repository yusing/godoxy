package middleware

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/yusing/godoxy/internal/routing"
	httpevents "github.com/yusing/goutils/events/http"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/http/httpheaders"
)

type (
	forwardAuthMiddleware struct {
		ForwardAuthMiddlewareOpts
	}

	ForwardAuthMiddlewareOpts struct {
		Route               string   `json:"route" validate:"required"`             // route name (alias), default: "tinyauth"
		AuthEndpoint        string   `json:"auth_endpoint" validate:"required,uri"` // default: "/api/auth/nginx"
		AuthResponseHeaders []string `json:"headers"`                               // additional headers to forward from auth server to upstream, e.g. ["Remote-User", "Remote-Name"]

		httpClient *http.Client
	}
)

var ForwardAuth = NewMiddleware[forwardAuthMiddleware]()

func (m *forwardAuthMiddleware) setup() {
	m.ForwardAuthMiddlewareOpts = ForwardAuthMiddlewareOpts{
		Route:               "tinyauth",
		AuthEndpoint:        "/api/auth/traefik",
		AuthResponseHeaders: []string{"Remote-User", "Remote-Name", "Remote-Email", "Remote-Groups"},
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			// do not follow redirects, we handle them in the middleware
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// before implements RequestModifier.
func (m *forwardAuthMiddleware) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	route, ok := routing.EntrypointFromCtx(r.Context()).HTTPRoutes().Get(m.Route)
	if !ok {
		ForwardAuth.LogWarn(r).Str("route", m.Route).Msg("forwardauth route not found")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}

	forwardAuthURL := *route.TargetURL()
	forwardAuthURL.Path = m.AuthEndpoint

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, forwardAuthURL.String(), nil)
	if err != nil {
		ForwardAuth.LogError(r).Err(err).Msg("failed to create request")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}

	xff, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		xff = r.RemoteAddr
	}

	proto := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		proto = "https"
	}

	req.Header = r.Header.Clone()
	req.Header.Set("X-Forwarded-For", xff)
	req.Header.Set("X-Forwarded-Proto", proto)
	req.Header.Set("X-Forwarded-Host", r.Host)
	req.Header.Set("X-Forwarded-Uri", r.URL.RequestURI())

	resp, err := m.httpClient.Do(req)
	if err != nil {
		ForwardAuth.LogError(r).Err(err).Msg("failed to connect to forwardauth server")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}

	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer httpevents.Blocked(r, "ForwardAuth", fmt.Sprintf("HTTP %d", resp.StatusCode))

		body, release, err := httputils.ReadAllBody(resp)
		defer release(body)

		if err != nil {
			ForwardAuth.LogError(r).Err(err).Msg("failed to read response body")
			w.WriteHeader(http.StatusInternalServerError)
			return false
		}
		httpheaders.CopyHeader(w.Header(), resp.Header)
		httpheaders.RemoveHopByHopHeaders(w.Header())

		isGet := r.Method == http.MethodGet
		isWS := httpheaders.IsWebsocket(r.Header)
		if !isGet || isWS {
			reqType := r.Method
			if isWS {
				reqType = "WebSocket"
			}
			ForwardAuth.LogWarn(r).Msgf(
				"[ForwardAuth] %s request rejected by auth upstream (HTTP %d).\nConsider adding bypass rule for this path if needed",
				reqType,
				resp.StatusCode,
			)
		}

		loc, err := resp.Location()
		if err != nil {
			if !errors.Is(err, http.ErrNoLocation) {
				ForwardAuth.LogError(r).Err(err).Msg("failed to get location")
				w.WriteHeader(http.StatusInternalServerError)
				return false
			}
		} else if loc := loc.String(); loc != "" {
			r.Header.Set("Location", loc)
		}
		w.WriteHeader(resp.StatusCode)

		_, err = w.Write(body)
		if err != nil {
			ForwardAuth.LogError(r).Err(err).Msg("failed to write response body")
		}
		return false
	}

	for _, h := range m.AuthResponseHeaders {
		if v := resp.Header.Get(h); v != "" {
			// NOTE: need to set the header to the original request to forward to upstream
			r.Header.Set(h, v)
		}
	}
	return true
}
