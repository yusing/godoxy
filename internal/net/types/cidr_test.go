package nettypes

import (
	"net"
	"testing"

	"github.com/yusing/godoxy/internal/net/hostaddr"
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

func TestCIDRParseHost(t *testing.T) {
	records := map[string][]string{"dyn.example.com": {"203.0.113.7", "2001:db8::7"}}
	hostaddr.UseStaticRecordsForTest(t, records)

	var c CIDR
	if err := c.Parse("host:dyn.example.com"); err != nil {
		t.Fatal(err)
	}
	if got := c.String(); got != "host:dyn.example.com" {
		t.Errorf("String() = %q", got)
	}
	if text, _ := c.MarshalText(); string(text) != "host:dyn.example.com" {
		t.Errorf("MarshalText() = %q", text)
	}
	if c.Host() == nil {
		t.Fatal("Host() = nil")
	}
	for ip, want := range map[string]bool{"203.0.113.7": true, "2001:db8::7": true, "203.0.113.8": false} {
		if got := c.Contains(net.ParseIP(ip)); got != want {
			t.Errorf("Contains(%s) = %v, want %v", ip, got, want)
		}
	}

	records["dyn.example.com"] = []string{"198.51.100.9"}
	hostaddr.RefreshAllForTest(t)
	if c.Contains(net.ParseIP("203.0.113.7")) || !c.Contains(net.ParseIP("198.51.100.9")) {
		t.Error("CIDR did not follow the new address")
	}

	if err := c.Parse("host:not a host"); err == nil {
		t.Error("Parse(host:not a host) succeeded")
	}
}
