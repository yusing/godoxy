package acl

import "net"

type ACL interface {
	IPAllowed(ip net.IP) bool
	WrapTCP(l net.Listener) net.Listener
	WrapUDP(l net.PacketConn) net.PacketConn
}
