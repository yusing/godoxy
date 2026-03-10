package stream

import (
	"fmt"
	"io"
	"net"

	"github.com/pires/go-proxyproto"
)

func writeProxyProtocolHeader(dst io.Writer, src net.Conn) error {
	srcAddr, ok := src.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("unexpected source address type %T", src.RemoteAddr())
	}
	dstAddr, ok := src.LocalAddr().(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("unexpected destination address type %T", src.LocalAddr())
	}

	header := &proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: transportProtocol(srcAddr, dstAddr),
		SourceAddr:        srcAddr,
		DestinationAddr:   dstAddr,
	}
	_, err := header.WriteTo(dst)
	return err
}

func transportProtocol(src, dst *net.TCPAddr) proxyproto.AddressFamilyAndProtocol {
	if src.IP.To4() != nil && dst.IP.To4() != nil {
		return proxyproto.TCPv4
	}
	return proxyproto.TCPv6
}
