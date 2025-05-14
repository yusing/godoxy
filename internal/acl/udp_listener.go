package acl

import (
	"net"
	"time"
)

type UDPListener struct {
	acl *Config
	lis net.PacketConn
}

func (c *Config) WrapUDP(lis net.PacketConn) net.PacketConn {
	if c == nil {
		return lis
	}
	return &UDPListener{
		acl: c,
		lis: lis,
	}
}

func (s *UDPListener) LocalAddr() net.Addr {
	return s.lis.LocalAddr()
}

func (s *UDPListener) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, addr, err := s.lis.ReadFrom(p)
		if err != nil {
			return n, addr, err
		}
		udpAddr, ok := addr.(*net.UDPAddr)
		if !ok {
			// Not a UDPAddr, drop
			continue
		}
		if !s.acl.IPAllowed(udpAddr.IP) {
			// Drop packet from disallowed IP
			continue
		}
		return n, addr, nil
	}
}

func (s *UDPListener) WriteTo(p []byte, addr net.Addr) (int, error) {
	for {
		n, err := s.lis.WriteTo(p, addr)
		if err != nil {
			return n, err
		}
		udpAddr, ok := addr.(*net.UDPAddr)
		if !ok {
			// Not a UDPAddr, drop
			continue
		}
		if !s.acl.IPAllowed(udpAddr.IP) {
			// Drop packet to disallowed IP
			continue
		}
		return n, nil
	}
}

func (s *UDPListener) SetDeadline(t time.Time) error {
	return s.lis.SetDeadline(t)
}

func (s *UDPListener) SetReadDeadline(t time.Time) error {
	return s.lis.SetReadDeadline(t)
}

func (s *UDPListener) SetWriteDeadline(t time.Time) error {
	return s.lis.SetWriteDeadline(t)
}

func (s *UDPListener) Close() error {
	return s.lis.Close()
}
