package acl

import (
	"bytes"
	"net"
	"strings"

	"github.com/yusing/godoxy/internal/maxmind"
	gperr "github.com/yusing/goutils/errs"
)

type MatcherFunc func(*maxmind.IPInfo) bool

type Matcher struct {
	match MatcherFunc
	raw   string
}

type Matchers []Matcher

const (
	MatcherTypeIP       = "ip"
	MatcherTypeCIDR     = "cidr"
	MatcherTypeTimeZone = "tz"
	MatcherTypeCountry  = "country"
)

// TODO: use this error in the future
//
//nolint:unused
var errMatcherFormat = gperr.Multiline().AddLines(
	"invalid matcher format, expect {type}:{value}",
	"Available types: ip|cidr|tz|country",
	"ip:127.0.0.1",
	"cidr:127.0.0.0/8",
	"tz:Asia/Shanghai",
	"country:GB",
)

var (
	errSyntax      = gperr.New("syntax error")
	errInvalidIP   = gperr.New("invalid IP")
	errInvalidCIDR = gperr.New("invalid CIDR")
)

func (matcher *Matcher) Parse(s string) error {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return errSyntax
	}
	matcher.raw = s

	switch parts[0] {
	case MatcherTypeIP:
		ip := net.ParseIP(parts[1])
		if ip == nil {
			return errInvalidIP
		}
		matcher.match = matchIP(ip)
	case MatcherTypeCIDR:
		_, net, err := net.ParseCIDR(parts[1])
		if err != nil {
			return errInvalidCIDR
		}
		matcher.match = matchCIDR(net)
	case MatcherTypeTimeZone:
		matcher.match = matchTimeZone(parts[1])
	case MatcherTypeCountry:
		matcher.match = matchISOCode(parts[1])
	default:
		return errSyntax
	}
	return nil
}

func (matchers Matchers) Match(ip *maxmind.IPInfo) bool {
	for _, m := range matchers {
		if m.match(ip) {
			return true
		}
	}
	return false
}

func (matchers Matchers) MarshalText() ([]byte, error) {
	if len(matchers) == 0 {
		return []byte("[]"), nil
	}
	var buf bytes.Buffer
	for _, m := range matchers {
		buf.WriteString(m.raw)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func matchIP(ip net.IP) MatcherFunc {
	return func(ip2 *maxmind.IPInfo) bool {
		return ip.Equal(ip2.IP)
	}
}

func matchCIDR(n *net.IPNet) MatcherFunc {
	return func(ip *maxmind.IPInfo) bool {
		return n.Contains(ip.IP)
	}
}

func matchTimeZone(tz string) MatcherFunc {
	return func(ip *maxmind.IPInfo) bool {
		city, ok := maxmind.LookupCity(ip)
		if !ok {
			return false
		}
		return city.Location.TimeZone == tz
	}
}

func matchISOCode(iso string) MatcherFunc {
	return func(ip *maxmind.IPInfo) bool {
		city, ok := maxmind.LookupCity(ip)
		if !ok {
			return false
		}
		return city.Country.IsoCode == iso
	}
}
