package acl

import (
	"net"
)

type TCPListener struct {
	acl *Config
	lis net.Listener
}

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
		return nil, nil
	}
	if !s.acl.IPAllowed(addr.IP) {
		c.Close()
		return nil, nil
	}
	return c, nil
}

func (s *TCPListener) Close() error {
	return s.lis.Close()
}
