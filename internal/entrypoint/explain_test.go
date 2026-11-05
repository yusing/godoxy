package entrypoint

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	aclpkg "github.com/yusing/godoxy/internal/acl"
	acl "github.com/yusing/godoxy/internal/acl/types"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/serialization"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/events"
	"github.com/yusing/goutils/http/reverseproxy"
	"github.com/yusing/goutils/pool"
)

// compiledRoute is a fake route that exposes a compiled route middleware chain.
type compiledRoute struct {
	*fakeHTTPRoute
	mid *middleware.Middleware
}

func (r *compiledRoute) CompiledMiddleware() *middleware.Middleware { return r.mid }

func newExplainEntrypoint(t *testing.T, mws []map[string]any) (*Entrypoint, *httpServer) {
	t.Helper()
	ep := NewTestEntrypoint(t, nil)
	ep.SetFindRouteDomains([]string{".example.com"})
	srv := newTestHTTPServer(t, ep)
	if mws != nil {
		require.NoError(t, ep.SetMiddlewares(mws))
	}
	return ep, srv
}

var tunnelChain = []map[string]any{
	{"use": "cloudflare_real_ip"},
	{"use": "cidr_whitelist", "allow": []string{"10.0.0.0/8"}},
}

func TestExplainDeniedTunnelVisitor(t *testing.T) {
	ep, srv := newExplainEntrypoint(t, tunnelChain)
	srv.AddRoute(newFakeHTTPRoute(t, "vscode", ""))

	res, err := ep.Explain(ExplainRequest{
		IP:      "127.0.0.1",
		Host:    "vscode.example.com",
		Scheme:  "http",
		Headers: map[string][]string{"CF-Connecting-IP": {"198.51.100.10"}},
	})
	require.NoError(t, err)
	require.Equal(t, "vscode", res.Route)
	require.Equal(t, ExplainDenied, res.Decision)
	require.Equal(t, "entrypoint[1]", res.DecidedBy)
	require.Equal(t, "HTTP 403: IP not allowed", res.Detail)
	require.Equal(t, "198.51.100.10", res.ClientIP)
	require.Len(t, res.Entrypoint, 2)
}

func TestExplainAllowedAndConditional(t *testing.T) {
	ep, srv := newExplainEntrypoint(t, tunnelChain)
	srv.AddRoute(newFakeHTTPRoute(t, "app", ""))

	mid, err := middleware.BuildMiddlewareFromMap("secure", map[string]middleware.OptionsRaw{
		"rate_limit": {"average": 10, "burst": 10},
	})
	require.NoError(t, err)
	srv.AddRoute(&compiledRoute{fakeHTTPRoute: newFakeHTTPRoute(t, "secure", ""), mid: mid})

	res, err := ep.Explain(ExplainRequest{IP: "10.1.2.3", Host: "app.example.com", Scheme: "http"})
	require.NoError(t, err)
	require.Equal(t, ExplainAllowed, res.Decision)
	require.Equal(t, "10.1.2.3", res.ClientIP)

	res, err = ep.Explain(ExplainRequest{IP: "10.1.2.3", Host: "secure.example.com", Scheme: "http"})
	require.NoError(t, err)
	require.Equal(t, ExplainConditional, res.Decision)
	require.Len(t, res.RouteMiddlewares, 1)
	require.Equal(t, middleware.ExplainNotEvaluated, res.RouteMiddlewares[0].Result)
}

func TestExplainRouteBypassOverlay(t *testing.T) {
	ep, srv := newExplainEntrypoint(t, tunnelChain)
	route := newFakeHTTPRoute(t, "plex", "")
	route.routeMiddlewares = map[string]types.LabelMap{
		"cidr_whitelist": {"bypass": []string{"remote 203.0.113.7"}},
	}
	srv.AddRoute(route)
	srv.AddRoute(newFakeHTTPRoute(t, "other", ""))

	res, err := ep.Explain(ExplainRequest{IP: "203.0.113.7", Host: "plex.example.com", Scheme: "http"})
	require.NoError(t, err)
	require.Equal(t, ExplainAllowed, res.Decision)
	require.Equal(t, middleware.ExplainBypassed, res.Entrypoint[1].Result)

	// the promoted bypass applies to plex only
	res, err = ep.Explain(ExplainRequest{IP: "203.0.113.7", Host: "other.example.com", Scheme: "http"})
	require.NoError(t, err)
	require.Equal(t, ExplainDenied, res.Decision)
}

func TestExplainNotFoundAndACL(t *testing.T) {
	ep, srv := newExplainEntrypoint(t, nil)
	srv.AddRoute(newFakeHTTPRoute(t, "app", ""))

	res, err := ep.Explain(ExplainRequest{IP: "10.1.2.3", Host: "missing.example.com", Scheme: "http"})
	require.NoError(t, err)
	require.Equal(t, ExplainNotFound, res.Decision)

	var cfg aclpkg.Config
	require.NoError(t, serialization.MapUnmarshalValidate(map[string]any{
		"default":     "deny",
		"allow_local": false,
		"allow":       []string{"ip:203.0.113.7"},
	}, &cfg))
	acl.SetCtx(ep.task, &cfg)

	res, err = ep.Explain(ExplainRequest{IP: "198.51.100.10", Host: "app.example.com", Scheme: "http"})
	require.NoError(t, err)
	require.Equal(t, ExplainDenied, res.Decision)
	require.Equal(t, "acl", res.DecidedBy)
	require.Equal(t, &ExplainACL{Allowed: false, Reason: "denied by default"}, res.ACL)

	res, err = ep.Explain(ExplainRequest{IP: "203.0.113.7", Host: "app.example.com", Scheme: "http"})
	require.NoError(t, err)
	require.Equal(t, ExplainAllowed, res.Decision)
	require.True(t, res.ACL.Allowed)
}

func TestExplainValidatesInput(t *testing.T) {
	ep, _ := newExplainEntrypoint(t, nil)
	var cfg aclpkg.Config
	require.NoError(t, serialization.MapUnmarshalValidate(map[string]any{"default": "deny", "allow_local": false, "allow": []string{"ip:203.0.113.7"}}, &cfg))
	acl.SetCtx(ep.task, &cfg)
	for _, req := range []ExplainRequest{
		{IP: "not-an-ip", Host: "app.example.com"},
		{IP: "10.1.2.3"},
		{IP: "10.1.2.3", Host: "app.example.com", Scheme: "ftp"},
		{IP: "10.1.2.3", Host: "app example.com"},
		{IP: "10.1.2.3", Host: "app.example.com", Method: "BAD METHOD"},
		{IP: "10.1.2.3", Host: "app.example.com", Path: "/%zz"},
		{IP: "10.1.2.3", Host: "app.example.com/injected"},
		{IP: "10.1.2.3", Host: "app.example.com?query"},
		{IP: "10.1.2.3", Host: "user@app.example.com"},
		{IP: "10.1.2.3", Host: "app.example.com:65536"},
		{IP: "10.1.2.3", Host: "app.example.com", Headers: http.Header{"Bad Name": {"value"}}},
		{IP: "10.1.2.3", Host: "app.example.com", Headers: http.Header{"Good-Name": {"value\r\nInjected: yes"}}},
	} {
		require.NotPanics(t, func() {
			_, err := ep.Explain(req)
			require.ErrorIs(t, err, ErrInvalidExplainRequest, "%+v", req)
		})
	}
}

func TestExplainOverlayPreservesResolvedIP(t *testing.T) {
	ep, srv := newExplainEntrypoint(t, tunnelChain)
	base := newFakeHTTPRoute(t, "app", "")
	base.routeMiddlewares = map[string]types.LabelMap{"cidr_whitelist": {"allow": []string{"198.51.100.0/24"}, "bypass": []string{"path /public"}}}
	mid, err := middleware.BuildMiddlewareFromMap("app", base.routeMiddlewares)
	require.NoError(t, err)
	base.handler = func(w http.ResponseWriter, r *http.Request) {
		mid.ServeHTTP(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }, w, r)
	}
	srv.AddRoute(&compiledRoute{fakeHTTPRoute: base, mid: mid})
	req := httptest.NewRequest("GET", "http://app.example.com/public", nil)
	req.RemoteAddr = "10.1.2.3:1000"
	req.Header.Set("CF-Connecting-IP", "198.51.100.10")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, 204, rec.Code)
	res, err := ep.Explain(ExplainRequest{IP: "10.1.2.3", Host: "app.example.com", Path: "/public", Scheme: "http", Headers: http.Header{"Cf-Connecting-Ip": {"198.51.100.10"}}})
	require.NoError(t, err)
	require.Equal(t, ExplainAllowed, res.Decision, "live status=%d explain=%+v steps=%+v", rec.Code, res, res.RouteMiddlewares)
}

func TestExplainSchemeSelectsActualListener(t *testing.T) {
	ep, httpSrv := newExplainEntrypoint(t, nil)
	httpsSrv := &httpServer{ep: ep, addr: common.ProxyHTTPSAddr, proto: HTTPProtoHTTPS, routes: pool.New[routing.HTTPRoute]("test-explain-https", "routes")}
	httpsSrv.AddRoute(newFakeHTTPRoute(t, "app", ""))
	ep.servers.Store(common.ProxyHTTPSAddr, httpsSrv)
	rec := httptest.NewRecorder()
	httpSrv.ServeHTTP(rec, httptest.NewRequest("GET", "http://app.example.com/", nil))
	require.Equal(t, 404, rec.Code)
	res, err := ep.Explain(ExplainRequest{IP: "10.1.2.3", Host: "app.example.com", Scheme: "http"})
	require.NoError(t, err)
	require.Equal(t, ExplainNotFound, res.Decision, "live HTTP status=%d explain=%+v", rec.Code, res)
}

func TestExplainMTLSIsConditional(t *testing.T) {
	ep, srv := newExplainEntrypoint(t, nil)
	srv.addr = common.ProxyHTTPSAddr
	srv.proto = HTTPProtoHTTPS
	ep.servers.Store(common.ProxyHTTPSAddr, srv)
	require.NoError(t, ep.SetInboundMTLSProfiles(map[string]types.InboundMTLSProfile{"secure": {UseSystemCAs: true}}))
	srv.AddRoute(newFakeHTTPRoute(t, "app", "secure"))
	cfg := srv.mutateServerTLSConfig(&tls.Config{})
	actual, err := cfg.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "app.example.com"})
	require.NoError(t, err)
	require.Equal(t, tls.RequireAndVerifyClientCert, actual.ClientAuth)
	res, err := ep.Explain(ExplainRequest{IP: "10.1.2.3", Host: "app.example.com", Scheme: "https"})
	require.NoError(t, err)
	require.Equal(t, ExplainConditional, res.Decision, "live TLS requires client cert, explain=%+v", res)
}

func TestExplainRouteResolutionInternalError(t *testing.T) {
	ep, srv := newExplainEntrypoint(t, nil)
	srv.addr = common.ProxyHTTPSAddr
	srv.proto = HTTPProtoHTTPS
	ep.servers.Store(common.ProxyHTTPSAddr, srv)
	srv.AddRoute(newFakeHTTPRoute(t, "app", "missing"))
	res, err := ep.Explain(ExplainRequest{IP: "10.1.2.3", Host: "app.example.com", Scheme: "https"})
	require.Error(t, err, "unknown mTLS profile should propagate internal failure, got %+v", res)
}

func TestExplainListenerAuthorityPort(t *testing.T) {
	ep, srv := newExplainEntrypoint(t, nil)
	srv.addr = ":8443"
	srv.proto = HTTPProtoHTTPS
	ep.servers.Store(srv.addr, srv)
	srv.AddRoute(newFakeHTTPRoute(t, "app", ""))
	for _, tc := range []struct {
		host, scheme string
		want         ExplainDecision
	}{
		{"APP.Example.COM:8443", "https", ExplainAllowed},
		{"app.example.com:443", "https", ExplainNotFound},
		{"app.example.com:8443", "http", ExplainNotFound},
		{"app.example.com", "https", ExplainNotFound},
	} {
		res, err := ep.Explain(ExplainRequest{IP: "10.1.2.3", Host: tc.host, Scheme: tc.scheme, Method: "CUSTOM", Path: "/valid%20path?q=value"})
		require.NoError(t, err)
		require.Equal(t, tc.want, res.Decision, "%+v", tc)
	}
}

type explainLoadBalancerRoute struct{ *compiledRoute }

func (*explainLoadBalancerRoute) ReverseProxy() *reverseproxy.ReverseProxy { return nil }

func TestExplainLoadBalancerIsConditional(t *testing.T) {
	ep, srv := newExplainEntrypoint(t, nil)
	srv.AddRoute(&explainLoadBalancerRoute{&compiledRoute{fakeHTTPRoute: newFakeHTTPRoute(t, "lb", "")}})
	res, err := ep.Explain(ExplainRequest{IP: "10.1.2.3", Host: "lb.example.com", Scheme: "http"})
	require.NoError(t, err)
	require.Equal(t, ExplainConditional, res.Decision)
	require.Empty(t, res.RouteMiddlewares)
}

func TestExplainGlobalMTLSAndNoEvents(t *testing.T) {
	ep, srv := newExplainEntrypoint(t, tunnelChain)
	history := events.NewHistory()
	events.SetCtx(ep.task, history)
	srv.AddRoute(newFakeHTTPRoute(t, "app", ""))
	res, err := ep.Explain(ExplainRequest{IP: "198.51.100.10", Host: "app.example.com", Scheme: "http"})
	require.NoError(t, err)
	require.Equal(t, ExplainDenied, res.Decision)
	require.NotEmpty(t, res.Notes)
	snapshot, _, cancel := history.SnapshotAndListen()
	cancel()
	require.Empty(t, snapshot, "dry run must not emit blocked events")
	ep.cfg = &Config{InboundMTLSProfile: "global"}
	require.NoError(t, ep.SetInboundMTLSProfiles(map[string]types.InboundMTLSProfile{"global": {UseSystemCAs: true}}))
	srv.addr = common.ProxyHTTPSAddr
	srv.proto = HTTPProtoHTTPS
	ep.servers.Store(srv.addr, srv)
	res, err = ep.Explain(ExplainRequest{IP: "198.51.100.10", Host: "app.example.com", Scheme: "https"})
	require.NoError(t, err)
	require.Equal(t, ExplainConditional, res.Decision)
	require.Equal(t, "tls", res.DecidedBy)
	require.Empty(t, res.Entrypoint)
	ep.cfg = &Config{InboundMTLSProfile: "missing"}
	_, err = ep.Explain(ExplainRequest{IP: "198.51.100.10", Host: "app.example.com", Scheme: "https"})
	require.Error(t, err)
}

func TestExplainOverlayPreservesChangedHost(t *testing.T) {
	ep, srv := newExplainEntrypoint(t, []map[string]any{
		{"use": "modify_request", "set_headers": map[string]string{"Host": "rewritten.example.com"}},
		{"use": "cidr_whitelist", "allow": []string{"10.0.0.0/8"}},
	})
	base := newFakeHTTPRoute(t, "app", "")
	base.routeMiddlewares = map[string]types.LabelMap{"cidr_whitelist": {"allow": []string{"198.51.100.0/24"}, "bypass": []string{"host rewritten.example.com"}}}
	mid, err := middleware.BuildMiddlewareFromMap("app", base.routeMiddlewares)
	require.NoError(t, err)
	base.handler = func(w http.ResponseWriter, r *http.Request) {
		mid.ServeHTTP(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }, w, r)
	}
	srv.AddRoute(&compiledRoute{fakeHTTPRoute: base, mid: mid})
	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.RemoteAddr = "198.51.100.10:1234"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, 204, rec.Code)
	res, err := ep.Explain(ExplainRequest{IP: "198.51.100.10", Host: "app.example.com", Scheme: "http"})
	require.NoError(t, err)
	require.Equal(t, ExplainAllowed, res.Decision)
	require.Equal(t, "app", res.Route)
}

func TestExplainUnknownEntrypointStopsBeforeRoute(t *testing.T) {
	ep, srv := newExplainEntrypoint(t, []map[string]any{{"use": "forward_auth", "route": "auth", "auth_endpoint": "/"}, {"use": "cidr_whitelist", "allow": []string{"10.0.0.0/8"}}})
	mid, err := middleware.BuildMiddlewareFromMap("app", map[string]middleware.OptionsRaw{"cidr_whitelist": {"allow": []string{"10.0.0.0/8"}}})
	require.NoError(t, err)
	srv.AddRoute(&compiledRoute{fakeHTTPRoute: newFakeHTTPRoute(t, "app", ""), mid: mid})
	res, err := ep.Explain(ExplainRequest{IP: "198.51.100.10", Host: "app.example.com", Scheme: "http"})
	require.NoError(t, err)
	require.Equal(t, ExplainConditional, res.Decision)
	require.Len(t, res.Entrypoint, 1)
	require.Empty(t, res.RouteMiddlewares)
}
