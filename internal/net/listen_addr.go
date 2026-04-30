package netutils

import (
	"net"
	"strings"

	"github.com/yusing/godoxy/internal/common"
)

// SharedHTTPSListenAddr returns the configured HTTPS listener address when addr
// is equivalent to it.
func SharedHTTPSListenAddr(addr string) string {
	if IsSharedHTTPSListenAddr(addr) {
		return common.ProxyHTTPSAddr
	}
	return addr
}

// IsSharedHTTPSListenAddr reports whether addr is equivalent to the configured
// HTTPS listener address.
func IsSharedHTTPSListenAddr(addr string) bool {
	return listenAddrsEqual(addr, common.ProxyHTTPSAddr)
}

func listenAddrsEqual(addr, other string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr == other
	}
	otherHost, otherPort, err := net.SplitHostPort(other)
	if err != nil {
		return addr == other
	}
	if port != otherPort {
		return false
	}
	return host == otherHost || IsWildcardListenHost(host) && IsWildcardListenHost(otherHost)
}

// IsWildcardListenHost reports whether host means all local interfaces.
func IsWildcardListenHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return true
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsUnspecified()
}
