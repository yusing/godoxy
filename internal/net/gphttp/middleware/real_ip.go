package middleware

import (
	"net"
	"net/http"
	"strings"

	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/goutils/http/httpheaders"
)

// https://nginx.org/en/docs/http/ngx_http_realip_module.html

type (
	realIP     RealIPOpts
	RealIPOpts struct {
		// Header is the name of the header to use for the real client IP
		Header string `validate:"required"`
		// From is a list of Address / CIDRs to trust
		From []*nettypes.CIDR `validate:"required,min=1"`
		/*
			If recursive search is disabled,
			the original client address that matches one of the trusted addresses is replaced by
			the last address sent in the request header field defined by the Header field.
			If recursive search is enabled,
			the original client address that matches one of the trusted addresses is replaced by
			the last non-trusted address sent in the request header field.
		*/
		Recursive bool
	}
)

var (
	RealIP            = NewMiddleware[realIP]()
	realIPOptsDefault = RealIPOpts{
		Header: "X-Real-IP",
		From:   []*nettypes.CIDR{},
	}
)

// setup implements MiddlewareWithSetup.
func (ri *realIP) setup() {
	*ri = realIP(realIPOptsDefault)
}

// before implements RequestModifier.
func (ri *realIP) before(w http.ResponseWriter, r *http.Request) bool {
	ri.setRealIP(r)
	return true
}

func (ri *realIP) isInCIDRList(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, CIDR := range ri.From {
		if CIDR == nil {
			continue
		}
		if CIDR.Contains(ip) {
			return true
		}
	}
	// not in any CIDR
	return false
}

func (ri *realIP) setRealIP(req *http.Request) {
	// skip first if header is not present
	realIPs := req.Header.Values(ri.Header)
	if len(realIPs) == 0 {
		// try non-canonical key
		realIPs = req.Header[ri.Header]
	}

	if len(realIPs) == 0 {
		return
	}

	clientIPStr, clientPort, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		clientIPStr = req.RemoteAddr
	}

	clientIP := net.ParseIP(clientIPStr)
	if !ri.isInCIDRList(clientIP) {
		return
	}

	lastNonTrustedIP := ""

	if !ri.Recursive {
		lastNonTrustedIP = parseRealIPHeaderValue(realIPs[len(realIPs)-1])
	} else {
		for _, r := range realIPs {
			ip := parseRealIPHeaderValue(r)
			if ip == "" {
				continue
			}
			if !ri.isInCIDRList(net.ParseIP(ip)) {
				lastNonTrustedIP = ip
			}
		}
	}

	if lastNonTrustedIP == "" {
		return
	}

	if clientPort != "" {
		req.RemoteAddr = net.JoinHostPort(lastNonTrustedIP, clientPort)
	} else {
		req.RemoteAddr = lastNonTrustedIP
	}
	req.Header.Set(ri.Header, lastNonTrustedIP)
	req.Header.Set(httpheaders.HeaderXRealIP, lastNonTrustedIP)
}

func parseRealIPHeaderValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	return ip.String()
}
