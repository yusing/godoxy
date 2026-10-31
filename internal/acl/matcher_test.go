package acl

import (
	"net"
	"reflect"
	"testing"

	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/godoxy/internal/net/hostaddr"
	"github.com/yusing/godoxy/internal/serialization"
)

func TestMatchers(t *testing.T) {
	strMatchers := []string{
		"ip:127.0.0.1",
		"ip:2001:db8::1",
		"cidr:10.0.0.0/8",
		"cidr:2001:db8:abcd::/48",
	}

	var mathers Matchers
	err := serialization.Convert(reflect.ValueOf(strMatchers), reflect.ValueOf(&mathers), false)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"2001:db8::1", true},
		{"10.0.0.1", true},
		{"2001:db8:abcd::123", true},
		{"127.0.0.2", false},
		{"2001:db8::2", false},
		{"192.168.0.1", false},
		{"11.0.0.1", false},
		{"2001:db8:abce::1", false},
	}

	for _, test := range tests {
		ip := net.ParseIP(test.ip)
		if ip == nil {
			t.Fatalf("invalid ip: %s", test.ip)
		}

		got := mathers.Match(t.Context(), &maxmind.IPInfo{
			IP:  ip,
			Str: test.ip,
		})
		if got != test.want {
			t.Errorf("mathers.Match(%s) = %v, want %v", test.ip, got, test.want)
		}
	}
}

func TestMatcherRejectsMalformedNonIPValues(t *testing.T) {
	tests := []string{
		"tz:Asia/Shanghai:extra",
		"country:GB:extra",
	}

	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			var matcher Matcher
			if err := matcher.Parse(test); err == nil {
				t.Fatal("expected parse error")
			}
		})
	}
}

func TestGeographicMatchersReuseResolvedInfo(t *testing.T) {
	city := &maxmind.City{}
	city.Country.IsoCode = "BR"
	city.Location.TimeZone = "America/Sao_Paulo"
	for _, rules := range [][]string{
		{"country:CN", "country:BR"},
		{"tz:Asia/Shanghai", "country:BR"},
		{"country:CN", "tz:America/Sao_Paulo"},
	} {
		matchers := mustMatchers(t, rules...)
		info := &maxmind.IPInfo{City: city}
		if got := matchers.MatchedIndex(t.Context(), info); got != 1 {
			t.Errorf("MatchedIndex(%v) = %d, want 1", rules, got)
		}
	}
}

func TestHostMatcher(t *testing.T) {
	records := map[string][]string{"dino.dyndns.example": {"203.0.113.7"}}
	hostaddr.UseStaticRecordsForTest(t, records)

	var m Matchers
	err := serialization.Convert(reflect.ValueOf([]string{"host:dino.dyndns.example"}), reflect.ValueOf(&m), false)
	if err != nil {
		t.Fatal(err)
	}
	if !m.HasDynamic() {
		t.Error("host matcher must be dynamic")
	}
	if !m.Match(t.Context(), &maxmind.IPInfo{IP: net.ParseIP("203.0.113.7")}) {
		t.Error("host matcher did not match its address")
	}
	if m.Match(t.Context(), &maxmind.IPInfo{IP: net.ParseIP("198.51.100.9")}) {
		t.Error("host matcher matched another address")
	}

	var bad Matcher
	if err := bad.Parse("host:"); err == nil {
		t.Error("empty host accepted")
	}
}
