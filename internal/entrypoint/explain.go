package entrypoint

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	acl "github.com/yusing/godoxy/internal/acl/types"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/route/routes"
)

// ExplainRequest describes a hypothetical request to explain.
type ExplainRequest struct {
	// IP is the address the connection comes from (the peer, e.g. 127.0.0.1
	// for a local tunnel); real IP middlewares may rewrite it from headers.
	IP     string
	Host   string
	Path   string
	Method string
	// Scheme is "https" (default) or "http".
	Scheme  string
	Headers http.Header
}

// ExplainDecision is the overall outcome.
type ExplainDecision string

const (
	ExplainAllowed ExplainDecision = "allowed"
	ExplainDenied  ExplainDecision = "denied"
	// ExplainResponded: a middleware answered itself (e.g. redirect_http).
	ExplainResponded ExplainDecision = "responded"
	// ExplainConditional: nothing denied the request, but some middleware
	// could not be evaluated (auth, CrowdSec, rate limit).
	ExplainConditional ExplainDecision = "conditional"
	ExplainNotFound    ExplainDecision = "not_found"
)

// ExplainACL is the connection-level ACL decision for the peer IP.
type ExplainACL struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
} // @name ExplainACL

// ExplainResult describes how the entrypoint would handle an ExplainRequest.
type ExplainResult struct {
	Route            string                   `json:"route,omitempty"`
	ACL              *ExplainACL              `json:"acl,omitempty"`
	Entrypoint       []middleware.ExplainStep `json:"entrypoint,omitempty"`
	RouteMiddlewares []middleware.ExplainStep `json:"route_middlewares,omitempty"`
	// ClientIP is the client address after real IP middlewares.
	ClientIP string          `json:"client_ip"`
	Decision ExplainDecision `json:"decision"`
	// DecidedBy names the ACL or middleware step that produced Decision.
	DecidedBy string `json:"decided_by,omitempty"`
	Detail    string `json:"detail,omitempty"`
	// Notes lists what the dry run did not cover.
	Notes []string `json:"notes,omitempty"`
} // @name ExplainResult

// ErrInvalidExplainRequest wraps errors caused by the ExplainRequest itself
// (bad IP, missing host, bad scheme or method, unparsable URL).
var ErrInvalidExplainRequest = errors.New("invalid explain request")

// Explain dry-runs a request through the ACL, the entrypoint middleware chain
// (with route bypass overlays) and the route's middlewares, without reaching
// the upstream. Middlewares with side effects are not run; see
// middleware.Middleware.Explain.
func (ep *Entrypoint) Explain(req ExplainRequest) (*ExplainResult, error) {
	peer := net.ParseIP(req.IP)
	if peer == nil {
		return nil, fmt.Errorf("%w: ip must be an IP address, got %q", ErrInvalidExplainRequest, req.IP)
	}
	if req.Host == "" {
		return nil, fmt.Errorf("%w: host is required", ErrInvalidExplainRequest)
	}
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	if req.Path == "" || req.Path[0] != '/' {
		req.Path = "/" + req.Path
	}
	scheme := strings.ToLower(req.Scheme)
	if scheme == "" {
		scheme = "https"
	}
	if scheme != "https" && scheme != "http" {
		return nil, fmt.Errorf("%w: scheme must be http or https, got %q", ErrInvalidExplainRequest, req.Scheme)
	}

	result := &ExplainResult{ClientIP: peer.String()}

	if explainer, ok := acl.FromCtx(ep.task.Context()).(acl.Explainer); ok {
		allowed, reason := explainer.ExplainIP(peer)
		result.ACL = &ExplainACL{Allowed: allowed, Reason: reason}
		if !allowed {
			result.Decision, result.DecidedBy, result.Detail = ExplainDenied, "acl", reason
			return result, nil
		}
	}

	// A fresh context: no events history, so denials during the dry run do
	// not emit "blocked" events or notifications.
	r, err := http.NewRequestWithContext(context.Background(), req.Method, scheme+"://"+req.Host+req.Path, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidExplainRequest, err)
	}
	r.RequestURI = r.URL.RequestURI()
	r.RemoteAddr = net.JoinHostPort(peer.String(), "0")
	for k, v := range req.Headers {
		r.Header[http.CanonicalHeaderKey(k)] = v
	}
	if scheme == "https" {
		r.TLS = &tls.ConnectionState{ServerName: hostWithoutPort(req.Host)}
	}
	r = accesslog.WithRequestInfo(r)

	srv := ep.explainServer(scheme, req.Host)
	if srv == nil {
		result.Decision, result.Detail = ExplainNotFound, "no HTTP listener"
		return result, nil
	}
	route, err := srv.resolveRequestRoute(r)
	if err != nil {
		result.Decision, result.DecidedBy, result.Detail = ExplainDenied, "entrypoint", err.Error()
		return result, nil
	}
	if route == nil {
		result.Decision, result.Detail = ExplainNotFound, "no route matches host "+req.Host
		result.Notes = append(result.Notes, "not-found rules are not evaluated")
		return result, nil
	}
	result.Route = route.Name()
	r = routes.WithRouteContext(r, route)

	conditional := false
	collect := func(steps []middleware.ExplainStep, proceed bool) bool {
		for _, s := range steps {
			if s.ClientIP != "" {
				result.ClientIP = s.ClientIP
			}
			if s.Result == middleware.ExplainNotEvaluated {
				conditional = true
			}
		}
		if proceed || len(steps) == 0 {
			return true
		}
		last := steps[len(steps)-1]
		result.DecidedBy, result.Detail = last.Middleware, last.Detail
		result.Decision = ExplainDenied
		if last.Result == middleware.ExplainResponded {
			result.Decision = ExplainResponded
		}
		return false
	}

	// Mirrors httpServer.ServeHTTP: the route's bypass rules promoted into
	// the entrypoint chain, and the consumed route entries skipped at route
	// level.
	routeReq := r
	if ep.middleware != nil {
		mid := ep.middleware
		overlay, err := srv.getRouteEntrypointOverlay(route)
		if err != nil && !errors.Is(err, errNoRouteEntrypointOverlay) {
			return nil, err
		}
		if overlay != nil {
			mid = overlay.middleware
			if len(overlay.consumedBypass) > 0 || len(overlay.consumedMiddlewares) > 0 {
				routeReq = middleware.WithConsumedRouteOverlays(r, overlay.consumedBypass, overlay.consumedMiddlewares)
			}
		}
		var proceed bool
		result.Entrypoint, proceed = mid.Explain(r)
		if !collect(result.Entrypoint, proceed) {
			return result, nil
		}
	}

	if compiled, ok := route.(interface {
		CompiledMiddleware() *middleware.Middleware
	}); ok {
		if mid := compiled.CompiledMiddleware(); mid != nil {
			var proceed bool
			result.RouteMiddlewares, proceed = mid.Explain(routeReq)
			if !collect(result.RouteMiddlewares, proceed) {
				return result, nil
			}
		}
	} else if len(route.RouteMiddlewares()) > 0 {
		result.Notes = append(result.Notes, "route middlewares of this route type are not evaluated")
	}

	result.Notes = append(result.Notes, "route rules are not evaluated")
	if conditional {
		result.Decision = ExplainConditional
		result.Detail = "nothing denied the request; some middlewares were not evaluated"
	} else {
		result.Decision = ExplainAllowed
	}
	return result, nil
}

// explainServer returns the listener serving host: the default listener for
// scheme if it has a matching route, else any listener that does, else the
// default listener (for not-found handling).
func (ep *Entrypoint) explainServer(scheme, host string) *httpServer {
	addr := common.ProxyHTTPSAddr
	if scheme == "http" {
		addr = common.ProxyHTTPAddr
	}
	def, hasDefault := ep.servers.Load(addr)
	if hasDefault && def.FindRoute(host) != nil {
		return def
	}
	for _, srv := range ep.servers.Range {
		if srv.FindRoute(host) != nil {
			return srv
		}
	}
	if hasDefault {
		return def
	}
	for _, srv := range ep.servers.Range {
		return srv
	}
	return nil
}
