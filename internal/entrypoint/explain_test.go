package entrypoint

import (
	"testing"

	"github.com/stretchr/testify/require"
	aclpkg "github.com/yusing/godoxy/internal/acl"
	acl "github.com/yusing/godoxy/internal/acl/types"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/serialization"
	"github.com/yusing/godoxy/internal/types"
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
	for _, req := range []ExplainRequest{
		{IP: "not-an-ip", Host: "app.example.com"},
		{IP: "10.1.2.3"},
		{IP: "10.1.2.3", Host: "app.example.com", Scheme: "ftp"},
	} {
		_, err := ep.Explain(req)
		require.Error(t, err)
	}
}
