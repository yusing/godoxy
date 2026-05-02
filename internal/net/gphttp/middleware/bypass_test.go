package middleware_test

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/entrypoint"
	. "github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/route"
	routeTypes "github.com/yusing/godoxy/internal/route/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/http/reverseproxy"
	expect "github.com/yusing/goutils/testing"
)

const (
	testPort = 18000
)

func noOpHandler(w http.ResponseWriter, r *http.Request) {}

func TestBypassCIDR(t *testing.T) {
	mr, err := ModifyRequest.New(map[string]any{
		"set_headers": map[string]string{
			"Test-Header": "test-value",
		},
		"bypass": []string{"remote 127.0.0.1/32"},
	})
	expect.NoError(t, err)

	tests := []struct {
		name         string
		remoteAddr   string
		expectBypass bool
	}{
		{"bypass", "127.0.0.1:8080", true},
		{"no_bypass", "192.168.1.1:8080", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
			req.RemoteAddr = test.remoteAddr
			recorder := httptest.NewRecorder()
			mr.ModifyRequest(noOpHandler, recorder, req)
			expect.NoError(t, err)
			if test.expectBypass {
				expect.Equal(t, req.Header.Get("Test-Header"), "")
			} else {
				expect.Equal(t, req.Header.Get("Test-Header"), "test-value")
			}
		})
	}
}

func TestBypassPath(t *testing.T) {
	mr, err := ModifyRequest.New(map[string]any{
		"bypass": []string{"path glob(/test/*)", "path /api"},
		"set_headers": map[string]string{
			"Test-Header": "test-value",
		},
	})
	expect.NoError(t, err)

	tests := []struct {
		name         string
		path         string
		expectBypass bool
	}{
		{"bypass", "/test/123", true},
		{"bypass2", "/test/123/456", true},
		{"bypass3", "/api", true},
		{"no_bypass", "/test1/123/456", false},
		{"no_bypass2", "/api/123", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com"+test.path, nil)
			recorder := httptest.NewRecorder()
			mr.ModifyRequest(noOpHandler, recorder, req)
			expect.NoError(t, err)
			if test.expectBypass {
				expect.Equal(t, req.Header.Get("Test-Header"), "")
			} else {
				expect.Equal(t, req.Header.Get("Test-Header"), "test-value")
			}
		})
	}
}

type fakeRoundTripper struct{}

func (f fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
		Header:     make(http.Header),
	}, nil
}

func TestReverseProxyBypass(t *testing.T) {
	url, err := url.Parse("http://example.com")
	expect.NoError(t, err)
	rp := reverseproxy.NewReverseProxy("test", url, fakeRoundTripper{})
	err = PatchReverseProxy(rp, map[string]OptionsRaw{
		"response": {
			"bypass": []string{"path glob(/test/*)", "path /api"},
			"set_headers": map[string]string{
				"Test-Header": "test-value",
			},
		},
	})
	expect.NoError(t, err)
	tests := []struct {
		name         string
		path         string
		expectBypass bool
	}{
		{"bypass", "/test/123", true},
		{"bypass2", "/test/123/456", true},
		{"bypass3", "/api", true},
		{"no_bypass", "/test1/123/456", false},
		{"no_bypass2", "/api/123", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com"+test.path, nil)
			recorder := httptest.NewRecorder()
			rp.ServeHTTP(recorder, req)
			if test.expectBypass {
				expect.Equal(t, recorder.Header().Get("Test-Header"), "")
			} else {
				expect.Equal(t, recorder.Header().Get("Test-Header"), "test-value")
			}
		})
	}
}

func TestBypassResponse(t *testing.T) {
	t.Run("req_rules", func(t *testing.T) {
		mr, err := ModifyResponse.New(map[string]any{
			"bypass": []string{"path glob(/test/*) | path /api"},
			"set_headers": map[string]string{
				"Test-Header": "test-value",
			},
		})
		expect.NoError(t, err)

		tests := []struct {
			name         string
			path         string
			expectBypass bool
		}{
			{"bypass", "/test/123", true},
			{"bypass2", "/test/123/456", true},
			{"bypass3", "/api", true},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "http://example.com"+test.path, nil)
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("test")),
					Request:    req,
					Header:     make(http.Header),
				}
				mErr := mr.ModifyResponse(resp)
				expect.NoError(t, mErr)
				if test.expectBypass {
					expect.Equal(t, resp.Header.Get("Test-Header"), "")
				} else {
					expect.Equal(t, resp.Header.Get("Test-Header"), "test-value")
				}
			})
		}
	})
	t.Run("res_rules", func(t *testing.T) {
		mr, err := ModifyResponse.New(map[string]any{
			"bypass": []string{"status 200"},
			"set_headers": map[string]string{
				"Test-Header": "test-value",
			},
		})
		expect.NoError(t, err)

		tests := []struct {
			name         string
			statusCode   int
			expectBypass bool
		}{
			{"bypass", 200, true},
			{"no_bypass", 201, false},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				resp := &http.Response{
					StatusCode: test.statusCode,
					Body:       io.NopCloser(strings.NewReader("test")),
					Header:     make(http.Header),
					Request:    httptest.NewRequest(http.MethodGet, "http://example.com", nil),
				}
				mErr := mr.ModifyResponse(resp)
				expect.NoError(t, mErr)
				if test.expectBypass {
					expect.Equal(t, resp.Header.Get("Test-Header"), "")
				} else {
					expect.Equal(t, resp.Header.Get("Test-Header"), "test-value")
				}
			})
		}
	})
}

func TestEntrypointBypassRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test"))
	}))
	defer srv.Close()

	url, err := url.Parse(srv.URL)
	expect.NoError(t, err)

	host, port, err := net.SplitHostPort(url.Host)
	expect.NoError(t, err)

	portInt, err := strconv.Atoi(port)
	expect.NoError(t, err)

	entry := entrypoint.NewTestEntrypoint(t, nil)
	r, err := route.NewStartedTestRoute(t, &route.Route{
		Alias:  "test-route",
		Scheme: routeTypes.SchemeHTTP,
		Host:   host,
		Port: routeTypes.Port{
			Listening: testPort,
			Proxy:     portInt,
		},
	})
	expect.NoError(t, err)

	err = entry.SetMiddlewares([]map[string]any{
		{
			"use":    "redirectHTTP",
			"bypass": []string{"route test-route"},
		},
		{
			"use": "response",
			"set_headers": map[string]string{
				"Test-Header": "test-value",
			},
		},
	})
	expect.NoError(t, err)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://test-route.example.com", nil)
	server, ok := entry.GetServer(":" + r.ListenURL().Port())
	if !ok {
		t.Fatal("server not found")
	}
	server.ServeHTTP(recorder, req)
	expect.Equal(t, recorder.Code, http.StatusOK, "should bypass http redirect")
	expect.Equal(t, recorder.Body.String(), "test")
	expect.Equal(t, recorder.Header().Get("Test-Header"), "test-value")
}

func TestEntrypointPromotesRouteBypassOverlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test"))
	}))
	defer srv.Close()

	targetURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	host, port, err := net.SplitHostPort(targetURL.Host)
	require.NoError(t, err)

	portInt, err := strconv.Atoi(port)
	require.NoError(t, err)

	entry := entrypoint.NewTestEntrypoint(t, nil)
	r, err := route.NewStartedTestRoute(t, &route.Route{
		Alias:  "test-route",
		Scheme: routeTypes.SchemeHTTP,
		Host:   host,
		Port: routeTypes.Port{
			Listening: testPort,
			Proxy:     portInt,
		},
		Middlewares: map[string]types.LabelMap{
			"redirectHTTP": {
				"bypass": `
- path glob(/public/*)
`[1:],
			},
		},
	})
	require.NoError(t, err)

	err = entry.SetMiddlewares([]map[string]any{
		{
			"use":    "redirectHTTP",
			"bypass": []string{"path /health"},
		},
	})
	require.NoError(t, err)

	server, ok := entry.GetServer(":" + r.ListenURL().Port())
	require.True(t, ok, "server not found")

	tests := []struct {
		name         string
		path         string
		expectStatus int
		expectBody   string
		expectLoc    string
	}{
		{
			name:         "existing_entrypoint_bypass_still_applies",
			path:         "/health",
			expectStatus: http.StatusOK,
			expectBody:   "test",
		},
		{
			name:         "route_bypass_is_promoted_to_entrypoint",
			path:         "/public/index.html",
			expectStatus: http.StatusOK,
			expectBody:   "test",
		},
		{
			name:         "non_matching_path_still_redirects",
			path:         "/private",
			expectStatus: http.StatusPermanentRedirect,
			expectLoc:    "https://test-route.example.com/private",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://test-route.example.com"+test.path, nil)
			server.ServeHTTP(recorder, req)
			assert.Equal(t, test.expectStatus, recorder.Code)
			if test.expectBody != "" {
				assert.Equal(t, test.expectBody, recorder.Body.String())
			}
			assert.Equal(t, test.expectLoc, recorder.Header().Get("Location"))
		})
	}
}

func TestRouteBypassWithoutMatchingEntrypointMiddlewareKeepsCurrentBehavior(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test"))
	}))
	defer srv.Close()

	targetURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	host, port, err := net.SplitHostPort(targetURL.Host)
	require.NoError(t, err)

	portInt, err := strconv.Atoi(port)
	require.NoError(t, err)

	entry := entrypoint.NewTestEntrypoint(t, nil)
	r, err := route.NewStartedTestRoute(t, &route.Route{
		Alias:  "test-route",
		Scheme: routeTypes.SchemeHTTP,
		Host:   host,
		Port: routeTypes.Port{
			Listening: testPort,
			Proxy:     portInt,
		},
		Middlewares: map[string]types.LabelMap{
			"redirectHTTP": {
				"bypass": `
- path glob(/public/*)
`[1:],
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, entry.SetMiddlewares([]map[string]any{{
		"use": "response",
		"set_headers": map[string]string{
			"X-Entrypoint-Overlay": "true",
		},
	}}))

	server, ok := entry.GetServer(":" + r.ListenURL().Port())
	require.True(t, ok, "server not found")

	t.Run("bypass_still_works_without_matching_entrypoint_middleware", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://test-route.example.com/public/index.html", nil)
		server.ServeHTTP(recorder, req)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "test", recorder.Body.String())
	})

	t.Run("route_middleware_still_redirects_for_non_matching_paths", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://test-route.example.com/private", nil)
		server.ServeHTTP(recorder, req)
		assert.Equal(t, http.StatusPermanentRedirect, recorder.Code)
		assert.Equal(t, "https://test-route.example.com/private", recorder.Header().Get("Location"))
	})
}
