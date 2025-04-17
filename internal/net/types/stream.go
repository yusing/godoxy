package gpnet

import (
	"net"
)

type (
	Stream interface {
		StreamListener
		Setup() error
		Handle(conn StreamConn) error
	}
	StreamListener interface {
		Addr() net.Addr
		Accept() (StreamConn, error)
		Close() error
	}
	StreamConn         any
	NetListenerWrapper struct {
		net.Listener
	}
)

func NetListener(l net.Listener) StreamListener {
	return NetListenerWrapper{Listener: l}
}

func (l NetListenerWrapper) Accept() (StreamConn, error) {
	return l.Listener.Accept()
}
