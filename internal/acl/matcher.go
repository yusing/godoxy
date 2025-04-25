package acl

import (
	"net"
	"strings"

	acl "github.com/yusing/go-proxy/internal/acl/types"
	"github.com/yusing/go-proxy/internal/gperr"
)

type matcher func(*acl.IPInfo) bool

const (
	MatcherTypeIP       = "ip"
	MatcherTypeCIDR     = "cidr"
	MatcherTypeTimeZone = "tz"
	MatcherTypeISO      = "iso"
)

var errMatcherFormat = gperr.Multiline().AddLines(
	"invalid matcher format, expect {type}:{value}",
	"Available types: ip|cidr|tz|iso",
	"ip:127.0.0.1",
	"cidr:127.0.0.0/8",
	"tz:Asia/Shanghai",
	"iso:GB",
)
var (
	errSyntax               = gperr.New("syntax error")
	errInvalidIP            = gperr.New("invalid IP")
	errInvalidCIDR          = gperr.New("invalid CIDR")
	errMaxMindNotConfigured = gperr.New("MaxMind not configured")
)

func (cfg *Config) parseMatcher(s string) (matcher, gperr.Error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return nil, errSyntax
	}

	switch parts[0] {
	case MatcherTypeIP:
		ip := net.ParseIP(parts[1])
		if ip == nil {
			return nil, errInvalidIP
		}
		return matchIP(ip), nil
	case MatcherTypeCIDR:
		_, net, err := net.ParseCIDR(parts[1])
		if err != nil {
			return nil, errInvalidCIDR
		}
		return matchCIDR(net), nil
	case MatcherTypeTimeZone:
		if cfg.MaxMind == nil {
			return nil, errMaxMindNotConfigured
		}
		return cfg.MaxMind.matchTimeZone(parts[1]), nil
	case MatcherTypeISO:
		if cfg.MaxMind == nil {
			return nil, errMaxMindNotConfigured
		}
		return cfg.MaxMind.matchISO(parts[1]), nil
	default:
		return nil, errSyntax
	}
}

func matchIP(ip net.IP) matcher {
	return func(ip2 *acl.IPInfo) bool {
		return ip.Equal(ip2.IP)
	}
}

func matchCIDR(n *net.IPNet) matcher {
	return func(ip *acl.IPInfo) bool {
		return n.Contains(ip.IP)
	}
}

func (cfg *MaxMindConfig) matchTimeZone(tz string) matcher {
	return func(ip *acl.IPInfo) bool {
		city, ok := cfg.lookupCity(ip)
		if !ok {
			return false
		}
		return city.Location.TimeZone == tz
	}
}

func (cfg *MaxMindConfig) matchISO(iso string) matcher {
	return func(ip *acl.IPInfo) bool {
		city, ok := cfg.lookupCity(ip)
		if !ok {
			return false
		}
		return city.Country.IsoCode == iso
	}
}
