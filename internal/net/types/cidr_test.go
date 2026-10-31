package nettypes

import (
	"net"
	"testing"
)

func TestCIDRParse(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		match   []string
		noMatch []string
	}{
		{in: "192.168.1.5", want: "192.168.1.5/32", match: []string{"192.168.1.5"}, noMatch: []string{"192.168.1.6"}},
		{in: "10.0.0.0/8", want: "10.0.0.0/8", match: []string{"10.255.0.1"}, noMatch: []string{"11.0.0.1"}},
		{in: "2001:db8::1", want: "2001:db8::1/128", match: []string{"2001:db8::1"}, noMatch: []string{"2001:db8::2", "2001:db8:ffff::99"}},
		{in: "2001:db8::/32", want: "2001:db8::/32", match: []string{"2001:db8:ffff::99"}, noMatch: []string{"2001:db9::1"}},
		{in: "::ffff:192.168.1.5", want: "192.168.1.5/32", match: []string{"192.168.1.5"}, noMatch: []string{"192.168.1.6"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			var c CIDR
			if err := c.Parse(tt.in); err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}
			if got := c.String(); got != tt.want {
				t.Errorf("Parse(%q) = %s, want %s", tt.in, got, tt.want)
			}
			for _, ip := range tt.match {
				if !c.Contains(net.ParseIP(ip)) {
					t.Errorf("%s should contain %s", tt.want, ip)
				}
			}
			for _, ip := range tt.noMatch {
				if c.Contains(net.ParseIP(ip)) {
					t.Errorf("%s should not contain %s", tt.want, ip)
				}
			}
		})
	}
}

func TestCIDRParseInvalid(t *testing.T) {
	for _, in := range []string{"", "abcd", "192.168.1.300", "10.0.0.0/33", "2001:db8::/129"} {
		var c CIDR
		if err := c.Parse(in); err == nil {
			t.Errorf("Parse(%q) = %s, want error", in, c.String())
		}
	}
}
