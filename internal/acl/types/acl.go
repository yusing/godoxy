package acl

import "net"

type ACL interface {
	IPAllowed(ip net.IP) bool
	WrapTCP(l net.Listener) net.Listener
	WrapUDP(l net.PacketConn) net.PacketConn
}

// Explainer is implemented by ACLs that can report a decision and its reason
// without logging, notifying or caching.
type Explainer interface {
	ExplainIP(ip net.IP) (allowed bool, reason string)
}
