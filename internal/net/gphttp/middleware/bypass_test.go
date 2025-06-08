package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yusing/go-proxy/internal/entrypoint"
	. "github.com/yusing/go-proxy/internal/net/gphttp/middleware"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/route"
	routeTypes "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/task"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
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
			req := httptest.NewRequest("GET", "http://example.com", nil)
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
		"bypass": []string{"path /test/*", "path /api"},
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
			req := httptest.NewRequest("GET", "http://example.com"+test.path, nil)
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
	rp := reverseproxy.NewReverseProxy("test", nettypes.MustParseURL("http://example.com"), fakeRoundTripper{})
	err := PatchReverseProxy(rp, map[string]OptionsRaw{
		"response": {
			"bypass": "path /test/* | path /api",
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
			req := httptest.NewRequest("GET", "http://example.com"+test.path, nil)
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

func TestEntrypointBypassRoute(t *testing.T) {
	go http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test"))
	}))
	entry := entrypoint.NewEntrypoint()
	r := &route.Route{
		Alias: "test-route",
		Port: routeTypes.Port{
			Proxy: 8080,
		},
	}
	err := entry.SetMiddlewares([]map[string]any{
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

	err = r.Validate()
	expect.NoError(t, err)
	r.Start(task.RootTask("test", false))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test-route.example.com", nil)
	entry.ServeHTTP(recorder, req)
	expect.Equal(t, recorder.Code, http.StatusOK, "should bypass http redirect")
	expect.Equal(t, recorder.Body.String(), "test")
	expect.Equal(t, recorder.Header().Get("Test-Header"), "test-value")
}
