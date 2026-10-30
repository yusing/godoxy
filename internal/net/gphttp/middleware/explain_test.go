package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/logging/accesslog"
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

	require.True(t, proceed)
	require.Len(t, steps, 3)
	require.Equal(t, ExplainBypassed, steps[0].Result)
	require.Equal(t, ExplainNotEvaluated, steps[1].Result)
	require.Equal(t, "rateLimiter", steps[1].Type)
	require.Equal(t, ExplainPassed, steps[2].Result, "https request is not redirected")
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
		require.True(t, proceed)
	}
	rec := httptest.NewRecorder()
	chain.ServeHTTP(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }, rec,
		httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil))
	require.Equal(t, http.StatusOK, rec.Code, "dry runs must not use up the limiter")
}
