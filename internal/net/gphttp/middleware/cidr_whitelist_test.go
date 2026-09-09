package middleware

import (
	_ "embed"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/http/reverseproxy"
	expect "github.com/yusing/goutils/testing"
)

//go:embed test_data/cidr_whitelist_test.yml
var testCIDRWhitelistCompose []byte
var deny, accept *Middleware

func TestCIDRWhitelistValidation(t *testing.T) {
	const testMessage = "test-message"
	t.Run("valid", func(t *testing.T) {
		_, err := CIDRWhiteList.New(OptionsRaw{
			"allow":   []string{"192.168.2.100/32"},
			"message": testMessage,
		})
		expect.NoError(t, err)
		_, err = CIDRWhiteList.New(OptionsRaw{
			"allow":   []string{"192.168.2.100/32"},
			"message": testMessage,
			"status":  403,
		})
		expect.NoError(t, err)
		_, err = CIDRWhiteList.New(OptionsRaw{
			"allow":       []string{"192.168.2.100/32"},
			"message":     testMessage,
			"status_code": 403,
		})
		expect.NoError(t, err)
	})
	for _, opts := range []OptionsRaw{nil, {"message": testMessage}, {"allow": []string{}}} {
		t.Run("empty allow denies requests", func(t *testing.T) {
			mid, err := CIDRWhiteList.New(opts)
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
			req.RemoteAddr = "192.168.2.100:1234"
			require.False(t, mid.TryModifyRequest(rec, req))
			require.Equal(t, http.StatusForbidden, rec.Code)
		})
	}

	t.Run("invalid cidr", func(t *testing.T) {
		_, err := CIDRWhiteList.New(OptionsRaw{
			"allow":   []string{"192.168.2.100/123"},
			"message": testMessage,
		})
		expect.ErrorT[*net.ParseError](t, err)
	})
	t.Run("invalid status code", func(t *testing.T) {
		_, err := CIDRWhiteList.New(OptionsRaw{
			"allow":       []string{"192.168.2.100/32"},
			"status_code": 600,
			"message":     testMessage,
		})
		expect.ErrorIs(t, serialization.ErrValidationError, err)
	})
}

func TestCIDRWhitelist(t *testing.T) {
	errs := gperr.NewBuilder("")
	mids := BuildMiddlewaresFromYAML("", testCIDRWhitelistCompose, &errs)
	expect.NoError(t, errs.Error())
	deny = mids["deny@file"]
	accept = mids["accept@file"]
	if deny == nil || accept == nil {
		panic("bug occurred")
	}

	t.Run("deny", func(t *testing.T) {
		t.Parallel()
		for range 10 {
			result, err := newMiddlewareTest(deny, nil)
			expect.NoError(t, err)
			expect.Equal(t, result.ResponseStatus, cidrWhitelistDefaults.StatusCode)
			expect.Equal(t, strings.TrimSpace(string(result.Data)), cidrWhitelistDefaults.Message)
		}
	})

	t.Run("accept", func(t *testing.T) {
		t.Parallel()
		for range 10 {
			result, err := newMiddlewareTest(accept, nil)
			expect.NoError(t, err)
			expect.Equal(t, result.ResponseStatus, http.StatusOK)
		}
	})
}

func TestCIDRWhitelistOverlayPromotion(t *testing.T) {
	routeOpts := map[string]OptionsRaw{
		"CIDRWhitelist": {"bypass": []string{"path /health"}},
	}
	target, err := url.Parse("http://example.com")
	require.NoError(t, err)
	rp := reverseproxy.NewReverseProxy("test-route", target, http.DefaultTransport)
	rp.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	require.NoError(t, PatchReverseProxy(rp, routeOpts))

	overlay, err := BuildEntrypointRouteOverlay("entrypoint", []map[string]any{{
		"use":   "CIDRWhitelist",
		"allow": []string{"192.168.2.0/24"},
	}}, "test-route", routeOpts)
	require.NoError(t, err)
	require.Contains(t, overlay.ConsumedMiddlewares, "cidrwhitelist")

	for _, tc := range []struct {
		name, route, path, ip string
		promote               bool
		status                int
	}{
		{"promoted bypass", "test-route", "/health", "203.0.113.1", true, http.StatusOK},
		{"unmatched path", "test-route", "/private", "203.0.113.1", true, http.StatusForbidden},
		{"other route", "other-route", "/health", "203.0.113.1", true, http.StatusForbidden},
		{"entrypoint allowed IP", "test-route", "/private", "192.168.2.100", true, http.StatusOK},
		{"local bypass without promotion", "test-route", "/health", "203.0.113.1", false, http.StatusOK},
		{"local deny without promotion", "test-route", "/private", "203.0.113.1", false, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com"+tc.path, nil)
			req.RemoteAddr = net.JoinHostPort(tc.ip, "1234")
			req = routes.WithRouteContext(req, fakeMiddlewareHTTPRoute{name: tc.route})
			rec := httptest.NewRecorder()
			if tc.promote {
				overlay.Middleware.ServeHTTP(func(w http.ResponseWriter, r *http.Request) {
					r = WithConsumedRouteOverlays(r, overlay.ConsumedBypass, overlay.ConsumedMiddlewares)
					rp.ServeHTTP(w, r)
				}, rec, req)
			} else {
				rp.ServeHTTP(rec, req)
			}
			require.Equal(t, tc.status, rec.Code)
		})
	}
}
