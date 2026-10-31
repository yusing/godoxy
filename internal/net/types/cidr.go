package nettypes

import (
	"fmt"
	"net"
	"strings"

	"github.com/yusing/godoxy/internal/net/hostaddr"
)

// CIDR is an address source in config: a CIDR, a bare IP (a single host), or
// "host:<name>", a hostname whose A/AAAA addresses are re-resolved in the
// background (see package hostaddr).
//
//nolint:recvcheck
type CIDR struct {
	net.IPNet

	host *hostaddr.Name
}

// NewCIDR returns a static CIDR for ipnet.
func NewCIDR(ipnet *net.IPNet) *CIDR {
	return &CIDR{IPNet: *ipnet}
}

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
	if name, ok := strings.CutPrefix(v, hostaddr.Prefix); ok {
		host, err := hostaddr.Get(name)
		if err != nil {
			return err
		}
		*cidr = CIDR{host: host}
		return nil
	}
	ipnet, err := ParseCIDROrIP(v)
	if err != nil {
		return fmt.Errorf("invalid CIDR, IP or host:<name> %q: %w", v, err)
	}
	*cidr = CIDR{IPNet: *ipnet}
	return nil
}

// Host returns the hostname source, nil for a static CIDR.
func (cidr CIDR) Host() *hostaddr.Name {
	return cidr.host
}

func (cidr CIDR) Contains(ip net.IP) bool {
	if cidr.host != nil {
		return cidr.host.Contains(ip)
	}
	return cidr.IPNet.Contains(ip)
}

func (cidr CIDR) String() string {
	if cidr.host != nil {
		return cidr.host.String()
	}
	return cidr.IPNet.String()
}

func (cidr CIDR) MarshalText() ([]byte, error) {
	return []byte(cidr.String()), nil
}
