package rules

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/yusing/godoxy/internal/route/routes"
	httputils "github.com/yusing/goutils/http"
)

const (
	VarRequestMethod      = "req_method"
	VarRequestScheme      = "req_scheme"
	VarRequestHost        = "req_host"
	VarRequestPort        = "req_port"
	VarRequestPath        = "req_path"
	VarRequestAddr        = "req_addr"
	VarRequestQuery       = "req_query"
	VarRequestURL         = "req_url"
	VarRequestURI         = "req_uri"
	VarRequestContentType = "req_content_type"
	VarRequestContentLen  = "req_content_length"
	VarRemoteHost         = "remote_host"
	VarRemotePort         = "remote_port"
	VarRemoteAddr         = "remote_addr"

	VarUpstreamName   = "upstream_name"
	VarUpstreamScheme = "upstream_scheme"
	VarUpstreamHost   = "upstream_host"
	VarUpstreamPort   = "upstream_port"
	VarUpstreamAddr   = "upstream_addr"
	VarUpstreamURL    = "upstream_url"

	VarRespContentType = "resp_content_type"
	VarRespContentLen  = "resp_content_length"
	VarRespStatusCode  = "status_code"
)

var staticReqVarSubsMap = map[string]reqVar{
	VarRequestMethod: {
		help: Help{
			command: "$" + VarRequestMethod,
			description: makeLines(
				"Inbound HTTP method (verb).",
				"Reads the request method exactly as the current handler sees it.",
				"$"+VarRequestMethod,
			),
		},
		get: func(req *http.Request) string { return req.Method },
	},
	VarRequestScheme: {
		help: Help{
			command: "$" + VarRequestScheme,
			description: makeLines(
				"Inbound request scheme.",
				"Returns https when TLS is present on the request; otherwise returns http.",
				"$"+VarRequestScheme,
			),
		},
		get: func(req *http.Request) string {
			if req.TLS != nil {
				return "https"
			}
			return "http"
		},
	},
	VarRequestHost: {
		help: Help{
			command: "$" + VarRequestHost,
			description: makeLines(
				"Inbound request host without port.",
				"Derived from the Host or authority component after stripping any explicit port.",
				"$"+VarRequestHost,
			),
		},
		get: func(req *http.Request) string {
			reqHost, _, err := net.SplitHostPort(req.Host)
			if err != nil {
				return req.Host
			}
			return reqHost
		},
	},
	VarRequestPort: {
		help: Help{
			command: "$" + VarRequestPort,
			description: makeLines(
				"Inbound request port.",
				"Split from the Host or authority component; empty when no explicit port was sent.",
				"$"+VarRequestPort,
			),
		},
		get: func(req *http.Request) string {
			_, reqPort, _ := net.SplitHostPort(req.Host)
			return reqPort
		},
	},
	VarRequestAddr: {
		help: Help{
			command: "$" + VarRequestAddr,
			description: makeLines(
				"Inbound request host as sent by the client.",
				"Includes the port when one is present in the Host or authority component.",
				"$"+VarRequestAddr,
			),
		},
		get: func(req *http.Request) string { return req.Host },
	},
	VarRequestPath: {
		help: Help{
			command: "$" + VarRequestPath,
			description: makeLines(
				"Inbound request path.",
				"Uses the URL path that rule matching and upstream forwarding operate on.",
				"$"+VarRequestPath,
			),
		},
		get: func(req *http.Request) string { return req.URL.Path },
	},
	VarRequestQuery: {
		help: Help{
			command: "$" + VarRequestQuery,
			description: makeLines(
				"Raw inbound query string.",
				"Returns the query portion without the leading ?.",
				"$"+VarRequestQuery,
			),
		},
		get: func(req *http.Request) string { return stripFragment(req.URL.RawQuery) },
	},
	VarRequestURL: {
		help: Help{
			command: "$" + VarRequestURL,
			description: makeLines(
				"Full inbound request URL.",
				"Includes scheme, host, path, and query when that information is available on the request URL.",
				"$"+VarRequestURL,
			),
		},
		get: func(req *http.Request) string { return req.URL.String() },
	},
	VarRequestURI: {
		help: Help{
			command: "$" + VarRequestURI,
			description: makeLines(
				"Request URI path plus query.",
				"Equivalent to the path and query string together, without any fragment.",
				"$"+VarRequestURI,
			),
		},
		get: func(req *http.Request) string { return stripFragment(req.URL.RequestURI()) },
	},
	VarRequestContentType: {
		help: Help{
			command: "$" + VarRequestContentType,
			description: makeLines(
				"Inbound request Content-Type header.",
				"Reads the current Content-Type header value from the request.",
				"$"+VarRequestContentType,
			),
		},
		get: func(req *http.Request) string { return req.Header.Get("Content-Type") },
	},
	VarRequestContentLen: {
		help: Help{
			command: "$" + VarRequestContentLen,
			description: makeLines(
				"Inbound request content length.",
				"String form of the request body content length. May be -1 when unknown.",
				"$"+VarRequestContentLen,
			),
		},
		get: func(req *http.Request) string { return strconv.FormatInt(req.ContentLength, 10) },
	},
	VarRemoteHost: {
		help: Help{
			command: "$" + VarRemoteHost,
			description: makeLines(
				"Remote client IP from RemoteAddr.",
				"Split from the request remote address. Empty when the address cannot be parsed into host and port.",
				"$"+VarRemoteHost,
			),
		},
		get: func(req *http.Request) string {
			clientIP, _, err := net.SplitHostPort(req.RemoteAddr)
			if err == nil {
				return clientIP
			}
			return ""
		},
	},
	VarRemotePort: {
		help: Help{
			command: "$" + VarRemotePort,
			description: makeLines(
				"Remote client port from RemoteAddr.",
				"Split from the request remote address. Empty when the address cannot be parsed into host and port.",
				"$"+VarRemotePort,
			),
		},
		get: func(req *http.Request) string {
			_, clientPort, err := net.SplitHostPort(req.RemoteAddr)
			if err == nil {
				return clientPort
			}
			return ""
		},
	},
	VarRemoteAddr: {
		help: Help{
			command: "$" + VarRemoteAddr,
			description: makeLines(
				"Raw remote client address.",
				"Usually the host:port string from the incoming request's RemoteAddr field.",
				"$"+VarRemoteAddr,
			),
		},
		get: func(req *http.Request) string { return req.RemoteAddr },
	},
	VarUpstreamName: {
		help: Help{
			command: "$" + VarUpstreamName,
			description: makeLines(
				"Selected upstream route name.",
				"Comes from route context after routing has selected the upstream target.",
				"$"+VarUpstreamName,
			),
		},
		get: routes.TryGetUpstreamName,
	},
	VarUpstreamScheme: {
		help: Help{
			command: "$" + VarUpstreamScheme,
			description: makeLines(
				"Selected upstream scheme.",
				"Examples include http, https, or h2c depending on the upstream target.",
				"$"+VarUpstreamScheme,
			),
		},
		get: routes.TryGetUpstreamScheme,
	},
	VarUpstreamHost: {
		help: Help{
			command: "$" + VarUpstreamHost,
			description: makeLines(
				"Selected upstream host.",
				"The host portion of the upstream target chosen for this request.",
				"$"+VarUpstreamHost,
			),
		},
		get: routes.TryGetUpstreamHost,
	},
	VarUpstreamPort: {
		help: Help{
			command: "$" + VarUpstreamPort,
			description: makeLines(
				"Selected upstream port.",
				"The port portion of the upstream target chosen for this request.",
				"$"+VarUpstreamPort,
			),
		},
		get: routes.TryGetUpstreamPort,
	},
	VarUpstreamAddr: {
		help: Help{
			command: "$" + VarUpstreamAddr,
			description: makeLines(
				"Selected upstream host and port.",
				"Combines the chosen upstream host and port as a single address string.",
				"$"+VarUpstreamAddr,
			),
		},
		get: routes.TryGetUpstreamAddr,
	},
	VarUpstreamURL: {
		help: Help{
			command: "$" + VarUpstreamURL,
			description: makeLines(
				"Selected upstream URL.",
				"Full upstream target URL resolved for the current request.",
				"$"+VarUpstreamURL,
			),
		},
		get: routes.TryGetUpstreamURL,
	},
}

var staticRespVarSubsMap = map[string]respVar{
	VarRespContentType: {
		help: Help{
			command: "$" + VarRespContentType,
			description: makeLines(
				"Current response Content-Type header.",
				"Available in post phase after upstream response headers and any earlier response mutations.",
				"$"+VarRespContentType,
			),
		},
		get: func(resp *httputils.ResponseModifier) string { return resp.Header().Get("Content-Type") },
	},
	VarRespContentLen: {
		help: Help{
			command: "$" + VarRespContentLen,
			description: makeLines(
				"Current response body length.",
				"Uses the buffered body size after response rewrites, otherwise the original or header Content-Length value.",
				"$"+VarRespContentLen,
			),
		},
		get: func(resp *httputils.ResponseModifier) string { return resp.ContentLengthStr() },
	},
	VarRespStatusCode: {
		help: Help{
			command: "$" + VarRespStatusCode,
			description: makeLines(
				"Current response status code.",
				"Available in post phase after upstream responds. Earlier post-phase rewrites can change the value seen here.",
				"$"+VarRespStatusCode,
			),
		},
		get: func(resp *httputils.ResponseModifier) string { return strconv.Itoa(resp.StatusCode()) },
	},
}

func stripFragment(s string) string {
	before, _, ok := strings.Cut(s, "#")
	if !ok {
		return s
	}
	return before
}
