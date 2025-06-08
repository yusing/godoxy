package middleware

import (
	"bytes"
	"net"
	"net/http"
	"slices"
	"testing"

	nettypes "github.com/yusing/go-proxy/internal/net/types"
	. "github.com/yusing/go-proxy/internal/utils/testing"
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
		ExpectNoError(t, err)
		ExpectEqual(t, mr.impl.(*modifyRequest).SetHeaders, opts["set_headers"].(map[string]string))
		ExpectEqual(t, mr.impl.(*modifyRequest).AddHeaders, opts["add_headers"].(map[string]string))
		ExpectEqual(t, mr.impl.(*modifyRequest).HideHeaders, opts["hide_headers"].([]string))
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
		ExpectNoError(t, err)
		ExpectEqual(t, result.RequestHeaders.Get("User-Agent"), "go-proxy/v0.5.0")
		ExpectEqual(t, result.RequestHeaders.Get("Host"), "test.example.com")
		ExpectTrue(t, slices.Contains(result.RequestHeaders.Values("Accept-Encoding"), "test-value"))
		ExpectEqual(t, result.RequestHeaders.Get("Accept"), "")

		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Req-Method"), "GET")
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Req-Scheme"), reqURL.Scheme)
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Req-Host"), reqURL.Hostname())
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Req-Port"), reqURL.Port())
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Req-Addr"), reqURL.Host)
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Req-Path"), reqURL.Path)
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Req-Query"), reqURL.RawQuery)
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Req-Url"), reqURL.String())
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Req-Uri"), reqURL.RequestURI())
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Req-Content-Type"), "application/json")
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Req-Content-Length"), "100")

		remoteHost, remotePort, _ := net.SplitHostPort(result.RemoteAddr)
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Remote-Host"), remoteHost)
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Remote-Port"), remotePort)
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Remote-Addr"), result.RemoteAddr)

		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Upstream-Scheme"), upstreamURL.Scheme)
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Upstream-Host"), upstreamURL.Hostname())
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Upstream-Port"), upstreamURL.Port())
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Upstream-Addr"), upstreamURL.Host)
		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Upstream-Url"), upstreamURL.String())

		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Header-Content-Type"), "application/json")

		ExpectEqual(t, result.RequestHeaders.Get("X-Test-Arg-Arg_1"), "b")
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
				ExpectNoError(t, err)
				ExpectEqual(t, result.RequestHeaders.Get("X-Test-Req-Path"), tt.expectedPath)
			})
		}
	})
}
