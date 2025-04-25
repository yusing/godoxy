package acl

import (
	"io"
	"net"
	"time"
)

type TCPListener struct {
	acl *Config
	lis net.Listener
}

type noConn struct{}

func (noConn) Read(b []byte) (int, error)         { return 0, io.EOF }
func (noConn) Write(b []byte) (int, error)        { return 0, io.EOF }
func (noConn) Close() error                       { return nil }
func (noConn) LocalAddr() net.Addr                { return nil }
func (noConn) RemoteAddr() net.Addr               { return nil }
func (noConn) SetDeadline(t time.Time) error      { return nil }
func (noConn) SetReadDeadline(t time.Time) error  { return nil }
func (noConn) SetWriteDeadline(t time.Time) error { return nil }

func (cfg *Config) WrapTCP(lis net.Listener) net.Listener {
	if cfg == nil {
		return lis
	}
	return &TCPListener{
		acl: cfg,
		lis: lis,
	}
}

func (s *TCPListener) Addr() net.Addr {
	return s.lis.Addr()
}

func (s *TCPListener) Accept() (net.Conn, error) {
	c, err := s.lis.Accept()
	if err != nil {
		return nil, err
	}
	addr, ok := c.RemoteAddr().(*net.TCPAddr)
	if !ok {
		// Not a TCPAddr, drop
		c.Close()
		return noConn{}, nil
	}
	if !s.acl.IPAllowed(addr.IP) {
		c.Close()
		return noConn{}, nil
	}
	return c, nil
}

func (s *TCPListener) Close() error {
	return s.lis.Close()
}
