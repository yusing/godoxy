package entrypoint

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	acl "github.com/yusing/godoxy/internal/acl/types"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/routing"
	strutils "github.com/yusing/goutils/strings"
	"golang.org/x/net/http/httpguts"
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
	ExplainAllowed ExplainDecision = "allowed" // The evaluated request phases passed; see Notes for limitations.
	ExplainDenied  ExplainDecision = "denied"
	// ExplainResponded: a middleware answered itself (e.g. redirect_http).
	ExplainResponded ExplainDecision = "responded"
	// ExplainConditional: evaluation depends on unmodeled behavior.
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
	Route            string                   `json:"route,omitempty" extensions:"x-omitempty"`
	ACL              *ExplainACL              `json:"acl,omitempty" extensions:"x-omitempty"`
	Entrypoint       []middleware.ExplainStep `json:"entrypoint,omitempty" extensions:"x-omitempty"`
	RouteMiddlewares []middleware.ExplainStep `json:"route_middlewares,omitempty" extensions:"x-omitempty"`
	// ClientIP is the client address after real IP middlewares.
	ClientIP string          `json:"client_ip"`
	Decision ExplainDecision `json:"decision"`
	// DecidedBy names the component or middleware step that stopped evaluation.
	DecidedBy string `json:"decided_by,omitempty" extensions:"x-omitempty"`
	Detail    string `json:"detail,omitempty" extensions:"x-omitempty"`
	// Notes lists what the dry run did not cover.
	Notes []string `json:"notes,omitempty" extensions:"x-omitempty"`
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

	// Parse authority and target separately so a path in Host cannot be
	// reinterpreted as part of the request URL.
	authority, err := url.Parse(scheme + "://" + req.Host)
	if err != nil || authority.Host != req.Host || authority.User != nil || authority.Path != "" || authority.RawQuery != "" || authority.Fragment != "" || !httpguts.ValidHostHeader(req.Host) || strings.ContainsAny(req.Host, "?#") {
		return nil, fmt.Errorf("%w: invalid host %q", ErrInvalidExplainRequest, req.Host)
	}
	if authority.Hostname() == "" {
		return nil, fmt.Errorf("%w: host is required", ErrInvalidExplainRequest)
	}
	if port := authority.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("%w: invalid port", ErrInvalidExplainRequest)
		}
	}
	target, err := url.ParseRequestURI(req.Path)
	if err != nil || !strings.HasPrefix(req.Path, "/") || strings.ContainsAny(req.Path, "\r\n#") {
		return nil, fmt.Errorf("%w: invalid request path", ErrInvalidExplainRequest)
	}
	target.Scheme, target.Host = scheme, req.Host
	// A fresh context has no event history, logging or notification hooks.
	r, err := http.NewRequestWithContext(context.Background(), req.Method, target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidExplainRequest, err)
	}
	for name, values := range req.Headers {
		if !httpguts.ValidHeaderFieldName(name) {
			return nil, fmt.Errorf("%w: invalid header name", ErrInvalidExplainRequest)
		}
		for _, value := range values {
			if !httpguts.ValidHeaderFieldValue(value) {
				return nil, fmt.Errorf("%w: invalid header value", ErrInvalidExplainRequest)
			}
			r.Header.Add(name, value)
		}
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
	r.RequestURI = r.URL.RequestURI()
	r.RemoteAddr = net.JoinHostPort(peer.String(), "0")
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
		if errors.Is(err, errSecureRouteRequiresSNI) || errors.Is(err, errSecureRouteMisdirected) {
			result.Decision, result.DecidedBy, result.Detail = ExplainDenied, "entrypoint", err.Error()
			return result, nil
		}
		return nil, err
	}
	// TLS verification precedes HTTP middleware and cannot be modeled without
	// a real handshake. Never report an mTLS-protected request as allowed.
	if r.TLS != nil {
		_, secure, err := srv.resolveInboundMTLSProfileForGlobal()
		if !secure && err == nil {
			_, secure, err = srv.resolveInboundMTLSProfileForRoute(route)
		}
		if err != nil {
			return nil, err
		}
		if secure {
			result.Decision, result.DecidedBy, result.Detail = ExplainConditional, "tls", "client certificate verification is not evaluated"
			if route != nil {
				result.Route = route.Name()
			}
			result.Notes = append(result.Notes, "TLS SNI is assumed to match Host; client certificates are not modeled")
			return result, nil
		}
	}
	if route == nil {
		result.Decision, result.Detail = ExplainNotFound, "no route matches host "+req.Host
		result.Notes = append(result.Notes, "short links and not-found rules are not evaluated")
		return result, nil
	}
	result.Route = route.Name()
	result.Notes = append(result.Notes, "route rules are not evaluated; they may change requests before route middlewares or produce a different response")
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
		if last.Result == middleware.ExplainNotEvaluated {
			result.Decision = ExplainConditional
		}
		if last.Result == middleware.ExplainResponded {
			result.Decision = ExplainResponded
		}
		return false
	}

	// Mirrors httpServer.ServeHTTP: the route's bypass rules promoted into
	// the entrypoint chain, and the consumed route entries skipped at route
	// level.
	var routeOverlay *routeEntrypointOverlay
	if ep.middleware != nil {
		mid := ep.middleware
		overlay, err := srv.getRouteEntrypointOverlay(route)
		if err != nil && !errors.Is(err, errNoRouteEntrypointOverlay) {
			return nil, err
		}
		if overlay != nil {
			mid = overlay.middleware
			routeOverlay = overlay
		}
		var proceed bool
		result.Entrypoint, proceed = mid.Explain(r)
		if !collect(result.Entrypoint, proceed) {
			return result, nil
		}
	}

	// Apply context copies only after entrypoint modifiers have changed Host,
	// RemoteAddr and the rest of the request, like ServeHTTP's next handler.
	routeReq := r
	if routeOverlay != nil {
		routeReq = middleware.WithConsumedRouteOverlays(r, routeOverlay.consumedBypass, routeOverlay.consumedMiddlewares)
	}
	if proxy, ok := route.(routing.ReverseProxyRoute); ok && proxy.ReverseProxy() == nil {
		result.Decision, result.Detail = ExplainConditional, "load-balancer backend selection and backend middleware are not evaluated"
		return result, nil
	}
	var routeMiddleware *middleware.Middleware
	if compiled, ok := route.(interface {
		CompiledMiddleware() *middleware.Middleware
	}); ok {
		routeMiddleware = compiled.CompiledMiddleware()
	}
	if routeMiddleware != nil {
		var proceed bool
		result.RouteMiddlewares, proceed = routeMiddleware.Explain(routeReq)
		if !collect(result.RouteMiddlewares, proceed) {
			return result, nil
		}
	} else {
		for name := range route.RouteMiddlewares() {
			if routeOverlay != nil {
				if _, consumed := routeOverlay.consumedMiddlewares[strutils.ToLowerNoSnake(name)]; consumed {
					continue
				}
			}
			conditional = true
			result.Notes = append(result.Notes, "route middlewares of this route type are not evaluated")
			break
		}
	}

	if conditional {
		result.Decision = ExplainConditional
		result.Detail = "some middleware phases were not evaluated"
	} else {
		result.Decision = ExplainAllowed
	}
	return result, nil
}

// explainServer selects only listeners matching the requested scheme and
// authority port, never a listener merely because it contains the route.
func (ep *Entrypoint) explainServer(scheme, host string) *httpServer {
	port := "443"
	addr := common.ProxyHTTPSAddr
	if scheme == "http" {
		port, addr = "80", common.ProxyHTTPAddr
	}
	authority := &url.URL{Host: host}
	if explicitPort := authority.Port(); explicitPort != "" {
		port = explicitPort
	}
	matches := func(srv *httpServer) bool {
		_, listenerPort, err := net.SplitHostPort(srv.addr)
		return err == nil && listenerPort == port && string(srv.proto) == scheme
	}
	if srv, ok := ep.servers.Load(addr); ok && matches(srv) {
		return srv
	}
	// Dedicated listeners use their configured authority, or a wildcard bind.
	for _, bind := range []string{authority.Hostname(), "", "0.0.0.0", "::"} {
		if srv, ok := ep.servers.Load(net.JoinHostPort(bind, port)); ok && matches(srv) {
			return srv
		}
	}
	return nil
}
