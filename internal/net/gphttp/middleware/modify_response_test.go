package middleware

import (
	"bytes"
	"net"
	"net/http"
	"net/url"
	"slices"
	"testing"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestModifyResponse(t *testing.T) {
	opts := OptionsRaw{
		"set_headers": map[string]string{
			"X-Test-Resp-Status":              VarRespStatusCode,
			"X-Test-Resp-Content-Type":        VarRespContentType,
			"X-Test-Resp-Content-Length":      VarRespContentLen,
			"X-Test-Resp-Header-Content-Type": "$resp_header(Content-Type)",

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
		mr, err := ModifyResponse.New(opts)
		expect.NoError(t, err)
		expect.Equal(t, mr.impl.(*modifyResponse).SetHeaders, opts["set_headers"].(map[string]string))
		expect.Equal(t, mr.impl.(*modifyResponse).AddHeaders, opts["add_headers"].(map[string]string))
		expect.Equal(t, mr.impl.(*modifyResponse).HideHeaders, opts["hide_headers"].([]string))
	})

	t.Run("response_headers", func(t *testing.T) {
		reqURL := expect.Must(url.Parse("https://my.app/?arg_1=b"))
		upstreamURL := expect.Must(url.Parse("http://test.example.com"))
		result, err := newMiddlewareTest(ModifyResponse, &testArgs{
			middlewareOpt: opts,
			reqURL:        reqURL,
			upstreamURL:   upstreamURL,
			body:          bytes.Repeat([]byte("a"), 100),
			headers: http.Header{
				"Content-Type": []string{"application/json"},
			},
			respHeaders: http.Header{
				"Content-Type": []string{"application/json"},
			},
			respBody:   bytes.Repeat([]byte("a"), 50),
			respStatus: http.StatusOK,
		})
		expect.NoError(t, err)
		expect.True(t, slices.Contains(result.ResponseHeaders.Values("Accept-Encoding"), "test-value"))
		expect.Equal(t, result.ResponseHeaders.Get("Accept"), "")

		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Resp-Status"), "200")
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Resp-Content-Type"), "application/json")
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Resp-Content-Length"), "50")
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Resp-Header-Content-Type"), "application/json")

		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Req-Method"), http.MethodGet)
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Req-Scheme"), reqURL.Scheme)
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Req-Host"), reqURL.Hostname())
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Req-Port"), reqURL.Port())
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Req-Addr"), reqURL.Host)
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Req-Path"), reqURL.Path)
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Req-Query"), reqURL.RawQuery)
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Req-Url"), reqURL.String())
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Req-Uri"), reqURL.RequestURI())
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Req-Content-Type"), "application/json")
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Req-Content-Length"), "100")

		remoteHost, remotePort, _ := net.SplitHostPort(result.RemoteAddr)
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Remote-Host"), remoteHost)
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Remote-Port"), remotePort)
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Remote-Addr"), result.RemoteAddr)

		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Upstream-Scheme"), upstreamURL.Scheme)
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Upstream-Host"), upstreamURL.Hostname())
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Upstream-Port"), upstreamURL.Port())
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Upstream-Addr"), upstreamURL.Host)
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Upstream-Url"), upstreamURL.String())

		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Header-Content-Type"), "application/json")
		expect.Equal(t, result.ResponseHeaders.Get("X-Test-Arg-Arg_1"), "b")
	})
}
