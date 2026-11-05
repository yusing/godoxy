package acl

import (
	"errors"
	"io/fs"
	"net"
	"time"

	"github.com/rs/zerolog/log"
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
			log.Error().Msgf("unexpected remote address type: %T, addr: %s", addr, addr.String())
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
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return 0, net.InvalidAddrError("not a UDP address")
	}
	if !s.acl.IPAllowed(udpAddr.IP) {
		return 0, fs.ErrPermission
	}
	return s.lis.WriteTo(p, addr)
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

type udpListener interface {
	SetReadBuffer(bytes int) error
	SetWriteBuffer(bytes int) error
}

var _ udpListener = (*net.UDPConn)(nil)

func (s *UDPListener) SetReadBuffer(bytes int) error {
	switch lis := s.lis.(type) {
	case udpListener:
		return lis.SetReadBuffer(bytes)
	default:
		return errors.New("not a UDPConn")
	}
}

func (s *UDPListener) SetWriteBuffer(bytes int) error {
	switch lis := s.lis.(type) {
	case udpListener:
		return lis.SetWriteBuffer(bytes)
	default:
		return errors.New("not a UDPConn")
	}
}

func (s *UDPListener) Close() error {
	return s.lis.Close()
}
