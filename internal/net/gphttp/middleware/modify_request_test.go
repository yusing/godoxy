package middleware

import (
	"bytes"
	"net"
	"net/http"
	"slices"
	"testing"

	nettypes "github.com/yusing/godoxy/internal/net/types"
	expect "github.com/yusing/goutils/testing"
)

func TestModifyRequest(t *testing.T) {
	opts := OptionsRaw{
		"set_headers": map[string]string{
			"User-Agent":                 "go-proxy/v0.5.0",
			"Host":                       VarUpstreamAddr,
			"X-Test-Req-Method":          VarRequestMethod,
			"X-Test-Req-Scheme":          VarRequestScheme,
			"X-Test-Req-Host":            VarRequestHost,
			"X-Test-Req-Port":            VarRequestPort,
			"X-Test-Req-Addr":            VarRequestAddr,
			"X-Test-Req-Path":            VarRequestPath,
			"X-Test-Req-Query":           VarRequestQuery,
			"X-Test-Req-Url":             VarRequestURL,
			"X-Test-Req-Uri":             VarRequestURI,
			"X-Test-Req-Content-Type":    VarRequestContentType,
			"X-Test-Req-Content-Length":  VarRequestContentLen,
			"X-Test-Remote-Host":         VarRemoteHost,
			"X-Test-Remote-Port":         VarRemotePort,
			"X-Test-Remote-Addr":         VarRemoteAddr,
			"X-Test-Upstream-Scheme":     VarUpstreamScheme,
			"X-Test-Upstream-Host":       VarUpstreamHost,
			"X-Test-Upstream-Port":       VarUpstreamPort,
			"X-Test-Upstream-Addr":       VarUpstreamAddr,
			"X-Test-Upstream-Url":        VarUpstreamURL,
			"X-Test-Header-Content-Type": "$header(Content-Type)",
			"X-Test-Arg-Arg_1":           "$arg(arg_1)",
		},
		"add_headers":  map[string]string{"Accept-Encoding": "test-value"},
		"hide_headers": []string{"Accept"},
	}

	t.Run("set_options", func(t *testing.T) {
		mr, err := ModifyRequest.New(opts)
		expect.NoError(t, err)
		expect.Equal(t, mr.impl.(*modifyRequest).SetHeaders, opts["set_headers"].(map[string]string))
		expect.Equal(t, mr.impl.(*modifyRequest).AddHeaders, opts["add_headers"].(map[string]string))
		expect.Equal(t, mr.impl.(*modifyRequest).HideHeaders, opts["hide_headers"].([]string))
	})

	t.Run("request_headers", func(t *testing.T) {
		reqURL := nettypes.MustParseURL("https://my.app/?arg_1=b")
		upstreamURL := nettypes.MustParseURL("http://test.example.com")
		result, err := newMiddlewareTest(ModifyRequest, &testArgs{
			middlewareOpt: opts,
			reqURL:        reqURL,
			upstreamURL:   upstreamURL,
			body:          bytes.Repeat([]byte("a"), 100),
			headers: http.Header{
				"Content-Type": []string{"application/json"},
			},
		})
		expect.NoError(t, err)
		expect.Equal(t, result.RequestHeaders.Get("User-Agent"), "go-proxy/v0.5.0")
		expect.Equal(t, result.RequestHeaders.Get("Host"), "test.example.com")
		expect.True(t, slices.Contains(result.RequestHeaders.Values("Accept-Encoding"), "test-value"))
		expect.Equal(t, result.RequestHeaders.Get("Accept"), "")

		expect.Equal(t, result.RequestHeaders.Get("X-Test-Req-Method"), "GET")
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Req-Scheme"), reqURL.Scheme)
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Req-Host"), reqURL.Hostname())
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Req-Port"), reqURL.Port())
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Req-Addr"), reqURL.Host)
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Req-Path"), reqURL.Path)
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Req-Query"), reqURL.RawQuery)
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Req-Url"), reqURL.String())
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Req-Uri"), reqURL.RequestURI())
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Req-Content-Type"), "application/json")
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Req-Content-Length"), "100")

		remoteHost, remotePort, _ := net.SplitHostPort(result.RemoteAddr)
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Remote-Host"), remoteHost)
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Remote-Port"), remotePort)
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Remote-Addr"), result.RemoteAddr)

		expect.Equal(t, result.RequestHeaders.Get("X-Test-Upstream-Scheme"), upstreamURL.Scheme)
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Upstream-Host"), upstreamURL.Hostname())
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Upstream-Port"), upstreamURL.Port())
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Upstream-Addr"), upstreamURL.Host)
		expect.Equal(t, result.RequestHeaders.Get("X-Test-Upstream-Url"), upstreamURL.String())

		expect.Equal(t, result.RequestHeaders.Get("X-Test-Header-Content-Type"), "application/json")

		expect.Equal(t, result.RequestHeaders.Get("X-Test-Arg-Arg_1"), "b")
	})

	t.Run("add_prefix", func(t *testing.T) {
		tests := []struct {
			name         string
			path         string
			expectedPath string
			upstreamURL  string
			addPrefix    string
		}{
			{
				name:         "no prefix",
				path:         "/foo",
				expectedPath: "/foo",
				upstreamURL:  "http://test.example.com",
			},
			{
				name:         "slash only",
				path:         "/",
				expectedPath: "/",
				upstreamURL:  "http://test.example.com",
				addPrefix:    "/", // should not change anything
			},
			{
				name:         "some prefix",
				path:         "/test",
				expectedPath: "/foo/test",
				upstreamURL:  "http://test.example.com",
				addPrefix:    "/foo",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				reqURL := nettypes.MustParseURL("https://my.app" + tt.path)
				upstreamURL := nettypes.MustParseURL(tt.upstreamURL)

				opts["add_prefix"] = tt.addPrefix
				result, err := newMiddlewareTest(ModifyRequest, &testArgs{
					middlewareOpt: opts,
					reqURL:        reqURL,
					upstreamURL:   upstreamURL,
				})
				expect.NoError(t, err)
				expect.Equal(t, result.RequestHeaders.Get("X-Test-Req-Path"), tt.expectedPath)
			})
		}
	})
}
