package nettypes

import (
	"net"
	"strings"
)

//nolint:recvcheck
type CIDR net.IPNet

func ParseCIDR(v string) (cidr CIDR, err error) {
	err = cidr.Parse(v)
	return
}

func (cidr *CIDR) Parse(v string) error {
	if !strings.Contains(v, "/") {
		v += "/32" // single IP
	}
	_, ipnet, err := net.ParseCIDR(v)
	if err != nil {
		return err
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
