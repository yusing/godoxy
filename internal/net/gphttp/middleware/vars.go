package middleware

import (
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
)

type (
	reqVarGetter  func(*http.Request) string
	respVarGetter func(*http.Response) string
)

var (
	reArg        = regexp.MustCompile(`\$arg\([\w-_]+\)`)
	reReqHeader  = regexp.MustCompile(`\$header\([\w-]+\)`)
	reRespHeader = regexp.MustCompile(`\$resp_header\([\w-]+\)`)
	reStatic     = regexp.MustCompile(`\$[\w_]+`)
)

const (
	VarRequestMethod      = "$req_method"
	VarRequestScheme      = "$req_scheme"
	VarRequestHost        = "$req_host"
	VarRequestPort        = "$req_port"
	VarRequestPath        = "$req_path"
	VarRequestAddr        = "$req_addr"
	VarRequestQuery       = "$req_query"
	VarRequestURL         = "$req_url"
	VarRequestURI         = "$req_uri"
	VarRequestContentType = "$req_content_type"
	VarRequestContentLen  = "$req_content_length"
	VarRemoteHost         = "$remote_host"
	VarRemotePort         = "$remote_port"
	VarRemoteAddr         = "$remote_addr"

	VarUpstreamName   = "$upstream_name"
	VarUpstreamScheme = "$upstream_scheme"
	VarUpstreamHost   = "$upstream_host"
	VarUpstreamPort   = "$upstream_port"
	VarUpstreamAddr   = "$upstream_addr"
	VarUpstreamURL    = "$upstream_url"

	VarRespContentType = "$resp_content_type"
	VarRespContentLen  = "$resp_content_length"
	VarRespStatusCode  = "$status_code"
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
	VarRequestQuery:       func(req *http.Request) string { return req.URL.RawQuery },
	VarRequestURL:         func(req *http.Request) string { return req.URL.String() },
	VarRequestURI:         func(req *http.Request) string { return req.URL.RequestURI() },
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
	VarUpstreamName:   func(req *http.Request) string { return req.Header.Get(httpheaders.HeaderUpstreamName) },
	VarUpstreamScheme: func(req *http.Request) string { return req.Header.Get(httpheaders.HeaderUpstreamScheme) },
	VarUpstreamHost:   func(req *http.Request) string { return req.Header.Get(httpheaders.HeaderUpstreamHost) },
	VarUpstreamPort:   func(req *http.Request) string { return req.Header.Get(httpheaders.HeaderUpstreamPort) },
	VarUpstreamAddr: func(req *http.Request) string {
		upHost := req.Header.Get(httpheaders.HeaderUpstreamHost)
		upPort := req.Header.Get(httpheaders.HeaderUpstreamPort)
		if upPort != "" {
			return upHost + ":" + upPort
		}
		return upHost
	},
	VarUpstreamURL: func(req *http.Request) string {
		upScheme := req.Header.Get(httpheaders.HeaderUpstreamScheme)
		if upScheme == "" {
			return ""
		}
		upHost := req.Header.Get(httpheaders.HeaderUpstreamHost)
		upPort := req.Header.Get(httpheaders.HeaderUpstreamPort)
		upAddr := upHost
		if upPort != "" {
			upAddr += ":" + upPort
		}
		return upScheme + "://" + upAddr
	},
}

var staticRespVarSubsMap = map[string]respVarGetter{
	VarRespContentType: func(resp *http.Response) string { return resp.Header.Get("Content-Type") },
	VarRespContentLen:  func(resp *http.Response) string { return strconv.FormatInt(resp.ContentLength, 10) },
	VarRespStatusCode:  func(resp *http.Response) string { return strconv.Itoa(resp.StatusCode) },
}

func varReplace(req *http.Request, resp *http.Response, s string) string {
	if req != nil {
		// Replace query parameters
		s = reArg.ReplaceAllStringFunc(s, func(match string) string {
			name := match[5 : len(match)-1]
			for k, v := range req.URL.Query() {
				if strings.EqualFold(k, name) {
					return v[0]
				}
			}
			return ""
		})

		// Replace request headers
		s = reReqHeader.ReplaceAllStringFunc(s, func(match string) string {
			header := http.CanonicalHeaderKey(match[8 : len(match)-1])
			return req.Header.Get(header)
		})
	}

	if resp != nil {
		// Replace response headers
		s = reRespHeader.ReplaceAllStringFunc(s, func(match string) string {
			header := http.CanonicalHeaderKey(match[13 : len(match)-1])
			return resp.Header.Get(header)
		})
	}

	// Replace static variables
	if req != nil {
		s = reStatic.ReplaceAllStringFunc(s, func(match string) string {
			if fn, ok := staticReqVarSubsMap[match]; ok {
				return fn(req)
			}
			return match
		})
	}

	if resp != nil {
		s = reStatic.ReplaceAllStringFunc(s, func(match string) string {
			if fn, ok := staticRespVarSubsMap[match]; ok {
				return fn(resp)
			}
			return match
		})
	}

	return s
}
