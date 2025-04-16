package gpnet

import (
	"fmt"
	"net"
)

type (
	Stream interface {
		fmt.Stringer
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
