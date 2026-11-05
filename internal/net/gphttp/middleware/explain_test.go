package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/routing"
)

func explainChain(t *testing.T, defs []map[string]any, remoteAddr string, headers map[string]string) ([]ExplainStep, bool, *http.Request) {
	t.Helper()
	chain, err := BuildMiddlewareFromChainRaw("entrypoint", defs)
	require.NoError(t, err)
	r := httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil)
	r.RemoteAddr = remoteAddr
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	r = accesslog.WithRequestInfo(r)
	steps, proceed := chain.Explain(r)
	return steps, proceed, r
}

func TestExplainTunnelVisitorDenied(t *testing.T) {
	steps, proceed, r := explainChain(t, []map[string]any{
		{"use": "cloudflare_real_ip"},
		{"use": "cidr_whitelist", "allow": []string{"10.0.0.0/8"}},
	}, "127.0.0.1:1234", map[string]string{"CF-Connecting-IP": "198.51.100.10"})

	require.False(t, proceed)
	require.Equal(t, []ExplainStep{
		{Middleware: "entrypoint[0]", Type: "cloudflareRealIP", Result: ExplainPassed, ClientIP: "198.51.100.10"},
		{Middleware: "entrypoint[1]", Type: "cidrWhitelist", Result: ExplainDenied, Detail: "HTTP 403: IP not allowed"},
	}, steps)
	require.Equal(t, "198.51.100.10:1234", r.RemoteAddr)
}

func TestExplainBypassAndNotEvaluated(t *testing.T) {
	steps, proceed, _ := explainChain(t, []map[string]any{
		{"use": "cidr_whitelist", "allow": []string{"10.0.0.0/8"}, "bypass": []string{"path /public/*", "remote 203.0.113.0/24"}},
		{"use": "rate_limit", "average": 1, "burst": 1},
		{"use": "redirect_http"},
	}, "203.0.113.7:1234", nil)

	require.False(t, proceed)
	require.Len(t, steps, 2)
	require.Equal(t, ExplainBypassed, steps[0].Result)
	require.Equal(t, ExplainNotEvaluated, steps[1].Result)
	require.Equal(t, "rateLimiter", steps[1].Type)
}

func TestExplainRedirectResponds(t *testing.T) {
	chain, err := BuildMiddlewareFromChainRaw("entrypoint", []map[string]any{{"use": "redirect_http"}})
	require.NoError(t, err)
	r := httptest.NewRequest(http.MethodGet, "http://app.example.com/x", nil)
	steps, proceed := chain.Explain(r)
	require.False(t, proceed)
	require.Equal(t, ExplainResponded, steps[0].Result)
	require.Contains(t, steps[0].Detail, "https://app.example.com")
}

func TestExplainDoesNotConsumeRateLimit(t *testing.T) {
	chain, err := BuildMiddlewareFromChainRaw("entrypoint", []map[string]any{
		{"use": "rate_limit", "average": 1, "burst": 1, "period": "1h"},
	})
	require.NoError(t, err)
	for range 5 {
		_, proceed := chain.Explain(httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil))
		require.False(t, proceed)
	}
	rec := httptest.NewRecorder()
	chain.ServeHTTP(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }, rec,
		httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil))
	require.Equal(t, http.StatusOK, rec.Code, "dry runs must not use up the limiter")
}

func TestExplainUnknownAuthChangesLaterBypass(t *testing.T) {
	var calls atomic.Int32
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Remote-User", "alice")
		w.WriteHeader(204)
	}))
	defer authServer.Close()
	chain, err := BuildMiddlewareFromChainRaw("entrypoint", []map[string]any{
		{"use": "forward_auth", "route": "auth", "auth_endpoint": "/"},
		{"use": "cidr_whitelist", "allow": []string{"10.0.0.0/8"}, "bypass": []string{"header Remote-User alice"}},
	})
	require.NoError(t, err)
	ep := forwardAuthTestEntrypoint{route: fakeMiddlewareHTTPRoute{name: "auth", targetURL: mustParseURL(authServer.URL)}}
	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.RemoteAddr = "198.51.100.10:1000"
	req = req.WithContext(context.WithValue(req.Context(), routing.EntrypointContextKey{}, ep))
	live := req.Clone(req.Context())
	rec := httptest.NewRecorder()
	chain.ServeHTTP(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }, rec, live)
	require.Equal(t, 204, rec.Code)
	require.Equal(t, int32(1), calls.Load())
	steps, proceed := chain.Explain(accesslog.WithRequestInfo(req))
	require.False(t, proceed)
	require.Len(t, steps, 1)
	require.Equal(t, ExplainNotEvaluated, steps[0].Result)
	require.Equal(t, int32(1), calls.Load(), "dry run must not contact auth")
}

func TestExplainResponseOnlyMiddleware(t *testing.T) {
	for _, bypass := range []bool{false, true} {
		opts := OptionsRaw{"set_headers": map[string]string{"X-Test": "value"}}
		if bypass {
			opts["bypass"] = []string{"path /public"}
		}
		chain, err := BuildMiddlewareFromMap("route", map[string]OptionsRaw{"modify_response": opts})
		require.NoError(t, err)
		steps, proceed := chain.Explain(httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil))
		require.True(t, proceed)
		require.Len(t, steps, 1)
		require.Equal(t, "modifyResponse", steps[0].Type)
		require.Equal(t, ExplainNotEvaluated, steps[0].Result)
	}
}

func TestExplainCloudflareDoesNotRefreshRanges(t *testing.T) {
	resetCloudflareRealIPTestState(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	loadCloudflareCIDRs = func() ([]*nettypes.CIDR, error) { <-release; return localCIDRs, nil }
	steps, proceed, r := explainChain(t, []map[string]any{{"use": "cloudflare_real_ip"}}, "127.0.0.1:1234", map[string]string{"CF-Connecting-IP": "198.51.100.10"})
	require.True(t, proceed)
	require.Equal(t, "198.51.100.10:1234", r.RemoteAddr)
	require.Equal(t, ExplainPassed, steps[0].Result)
	require.False(t, cfCIDRsRefreshing.Load(), "dry run must not start background network calls")
	require.True(t, cfCIDRsLastUpdate.Load().IsZero())
}
