package agentproxy

import (
	"net/http"
	"strconv"
)

const (
	HeaderXProxyHost                  = "X-Proxy-Host"
	HeaderXProxyHTTPS                 = "X-Proxy-Https"
	HeaderXProxySkipTLSVerify         = "X-Proxy-Skip-Tls-Verify"
	HeaderXProxyResponseHeaderTimeout = "X-Proxy-Response-Header-Timeout"
)

type AgentProxyHeaders struct {
	Host                  string
	IsHTTPS               bool
	SkipTLSVerify         bool
	ResponseHeaderTimeout int
}

func SetAgentProxyHeaders(r *http.Request, headers *AgentProxyHeaders) {
	r.Header.Set(HeaderXProxyHost, headers.Host)
	r.Header.Set(HeaderXProxyHTTPS, strconv.FormatBool(headers.IsHTTPS))
	r.Header.Set(HeaderXProxySkipTLSVerify, strconv.FormatBool(headers.SkipTLSVerify))
	r.Header.Set(HeaderXProxyResponseHeaderTimeout, strconv.Itoa(headers.ResponseHeaderTimeout))
}
