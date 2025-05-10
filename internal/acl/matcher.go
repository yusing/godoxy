package acl

import (
	"net"
	"strings"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/maxmind"
)

type Matcher func(*maxmind.IPInfo) bool

type Matchers []Matcher

const (
	MatcherTypeIP       = "ip"
	MatcherTypeCIDR     = "cidr"
	MatcherTypeTimeZone = "tz"
	MatcherTypeCountry  = "country"
)

var errMatcherFormat = gperr.Multiline().AddLines(
	"invalid matcher format, expect {type}:{value}",
	"Available types: ip|cidr|tz|country",
	"ip:127.0.0.1",
	"cidr:127.0.0.0/8",
	"tz:Asia/Shanghai",
	"country:GB",
)

var (
	errSyntax               = gperr.New("syntax error")
	errInvalidIP            = gperr.New("invalid IP")
	errInvalidCIDR          = gperr.New("invalid CIDR")
	errMaxMindNotConfigured = gperr.New("MaxMind not configured")
)

func ParseMatcher(s string) (Matcher, gperr.Error) {
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
		if !maxmind.HasInstance() {
			return nil, errMaxMindNotConfigured
		}
		return matchTimeZone(parts[1]), nil
	case MatcherTypeCountry:
		if !maxmind.HasInstance() {
			return nil, errMaxMindNotConfigured
		}
		return matchISOCode(parts[1]), nil
	default:
		return nil, errSyntax
	}
}

func (matchers Matchers) Match(ip *maxmind.IPInfo) bool {
	for _, m := range matchers {
		if m(ip) {
			return true
		}
	}
	return false
}

func matchIP(ip net.IP) Matcher {
	return func(ip2 *maxmind.IPInfo) bool {
		return ip.Equal(ip2.IP)
	}
}

func matchCIDR(n *net.IPNet) Matcher {
	return func(ip *maxmind.IPInfo) bool {
		return n.Contains(ip.IP)
	}
}

func matchTimeZone(tz string) Matcher {
	return func(ip *maxmind.IPInfo) bool {
		city, ok := maxmind.LookupCity(ip)
		if !ok {
			return false
		}
		return city.Location.TimeZone == tz
	}
}

func matchISOCode(iso string) Matcher {
	return func(ip *maxmind.IPInfo) bool {
		city, ok := maxmind.LookupCity(ip)
		if !ok {
			return false
		}
		return city.Country.IsoCode == iso
	}
}
