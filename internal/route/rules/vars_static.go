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

var staticReqVarSubsMap = map[string]reqVarGetter{
	VarRequestMethod: func(req *http.Request) string { return req.Method },
	VarRequestScheme: func(req *http.Request) string {
		if req.TLS != nil {
			return "https"
		}
		return "http"
	},
	VarRequestHost: func(req *http.Request) string {
		reqHost, _, err := net.SplitHostPort(req.Host)
		if err != nil {
			return req.Host
		}
		return reqHost
	},
	VarRequestPort: func(req *http.Request) string {
		_, reqPort, _ := net.SplitHostPort(req.Host)
		return reqPort
	},
	VarRequestAddr:        func(req *http.Request) string { return req.Host },
	VarRequestPath:        func(req *http.Request) string { return req.URL.Path },
	VarRequestQuery:       func(req *http.Request) string { return stripFragment(req.URL.RawQuery) },
	VarRequestURL:         func(req *http.Request) string { return req.URL.String() },
	VarRequestURI:         func(req *http.Request) string { return stripFragment(req.URL.RequestURI()) },
	VarRequestContentType: func(req *http.Request) string { return req.Header.Get("Content-Type") },
	VarRequestContentLen:  func(req *http.Request) string { return strconv.FormatInt(req.ContentLength, 10) },
	VarRemoteHost: func(req *http.Request) string {
		clientIP, _, err := net.SplitHostPort(req.RemoteAddr)
		if err == nil {
			return clientIP
		}
		return ""
	},
	VarRemotePort: func(req *http.Request) string {
		_, clientPort, err := net.SplitHostPort(req.RemoteAddr)
		if err == nil {
			return clientPort
		}
		return ""
	},
	VarRemoteAddr:     func(req *http.Request) string { return req.RemoteAddr },
	VarUpstreamName:   routes.TryGetUpstreamName,
	VarUpstreamScheme: routes.TryGetUpstreamScheme,
	VarUpstreamHost:   routes.TryGetUpstreamHost,
	VarUpstreamPort:   routes.TryGetUpstreamPort,
	VarUpstreamAddr:   routes.TryGetUpstreamAddr,
	VarUpstreamURL:    routes.TryGetUpstreamURL,
}

var staticRespVarSubsMap = map[string]respVarGetter{
	VarRespContentType: func(resp *httputils.ResponseModifier) string { return resp.Header().Get("Content-Type") },
	VarRespContentLen:  func(resp *httputils.ResponseModifier) string { return resp.ContentLengthStr() },
	VarRespStatusCode:  func(resp *httputils.ResponseModifier) string { return strconv.Itoa(resp.StatusCode()) },
}

func stripFragment(s string) string {
	idx := strings.IndexByte(s, '#')
	if idx == -1 {
		return s
	}
	return s[:idx]
}
