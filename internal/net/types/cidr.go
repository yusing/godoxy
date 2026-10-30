package nettypes

import (
	"fmt"
	"net"
	"strings"
)

//nolint:recvcheck
type CIDR net.IPNet

// ParseCIDROrIP parses a CIDR, or a bare IP as a single-host network:
// /32 for IPv4 and /128 for IPv6.
func ParseCIDROrIP(v string) (*net.IPNet, error) {
	if strings.Contains(v, "/") {
		_, ipnet, err := net.ParseCIDR(v)
		return ipnet, err
	}
	ip := net.ParseIP(v)
	if ip == nil {
		return nil, &net.ParseError{Type: "IP address", Text: v}
	}
	if ip4 := ip.To4(); ip4 != nil {
		return &net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}, nil
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, nil
}

func (cidr *CIDR) Parse(v string) error {
	ipnet, err := ParseCIDROrIP(v)
	if err != nil {
		return fmt.Errorf("invalid CIDR or IP %q: %w", v, err)
	}
	cidr.IP = ipnet.IP
	cidr.Mask = ipnet.Mask
	return nil
}

func (cidr CIDR) Contains(ip net.IP) bool {
	return (*net.IPNet)(&cidr).Contains(ip)
}

func (cidr CIDR) String() string {
	return (*net.IPNet)(&cidr).String()
}

func (cidr CIDR) MarshalText() ([]byte, error) {
	return []byte(cidr.String()), nil
}
