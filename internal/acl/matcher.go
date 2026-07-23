package acl

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"

	"github.com/yusing/godoxy/internal/maxmind"
	gperr "github.com/yusing/goutils/errs"
)

type MatcherFunc func(context.Context, *maxmind.IPInfo) bool

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
	errSyntax      = errors.New("syntax error")
	errInvalidIP   = errors.New("invalid IP")
	errInvalidCIDR = errors.New("invalid CIDR")
)

func (matcher *Matcher) Parse(s string) error {
	matcherType, matcherValue, ok := strings.Cut(s, ":")
	if !ok {
		return errSyntax
	}
	matcher.raw = s

	switch matcherType {
	case MatcherTypeIP:
		ip := net.ParseIP(matcherValue)
		if ip == nil {
			return errInvalidIP
		}
		matcher.match = matchIP(ip)
	case MatcherTypeCIDR:
		_, net, err := net.ParseCIDR(matcherValue)
		if err != nil {
			return errInvalidCIDR
		}
		matcher.match = matchCIDR(net)
	case MatcherTypeTimeZone:
		if strings.Contains(matcherValue, ":") {
			return errSyntax
		}
		matcher.match = matchTimeZone(matcherValue)
	case MatcherTypeCountry:
		if strings.Contains(matcherValue, ":") {
			return errSyntax
		}
		matcher.match = matchISOCode(matcherValue)
	default:
		return errSyntax
	}
	return nil
}

func (matchers Matchers) Match(ctx context.Context, ip *maxmind.IPInfo) bool {
	for _, m := range matchers {
		if m.match(ctx, ip) {
			return true
		}
	}
	return false
}

func (matchers Matchers) MatchedIndex(ctx context.Context, ip *maxmind.IPInfo) int {
	for i, m := range matchers {
		if m.match(ctx, ip) {
			return i
		}
	}
	return -1
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
	return func(_ context.Context, ip2 *maxmind.IPInfo) bool {
		return ip.Equal(ip2.IP)
	}
}

func matchCIDR(n *net.IPNet) MatcherFunc {
	return func(_ context.Context, ip *maxmind.IPInfo) bool {
		return n.Contains(ip.IP)
	}
}

func matchTimeZone(tz string) MatcherFunc {
	return func(ctx context.Context, ip *maxmind.IPInfo) bool {
		city, ok := maxmind.LookupCity(ctx, ip)
		if !ok {
			return false
		}
		return city.Location.TimeZone == tz
	}
}

func matchISOCode(iso string) MatcherFunc {
	return func(ctx context.Context, ip *maxmind.IPInfo) bool {
		city, ok := maxmind.LookupCity(ctx, ip)
		if !ok {
			return false
		}
		return city.Country.IsoCode == iso
	}
}
