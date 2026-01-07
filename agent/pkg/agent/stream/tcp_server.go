package stream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"

	ioutils "github.com/yusing/goutils/io"
)

type TCPServer struct {
	ctx      context.Context
	listener net.Listener
	connMgr  *ConnectionManager[net.Conn]
}

// NewTCPServerFromListener creates a TCP stream server from an already-prepared
// listener.
//
// The listener is expected to yield connections that are already secured (e.g.
// a TLS/mTLS listener, or pre-handshaked *tls.Conn). This is used when the agent
// multiplexes HTTPS and stream-tunnel traffic on the same port.
func NewTCPServerFromListener(ctx context.Context, listener net.Listener) *TCPServer {
	s := &TCPServer{
		ctx:      ctx,
		listener: listener,
	}
	s.connMgr = NewConnectionManager(s.createDestConnection)
	return s
}

func NewTCPServer(ctx context.Context, listener *net.TCPListener, caCert *x509.Certificate, serverCert *tls.Certificate) *TCPServer {
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*serverCert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{StreamALPN},
	}

	tcpListener := tls.NewListener(listener, tlsConfig)
	return NewTCPServerFromListener(ctx, tcpListener)
}

func (s *TCPServer) Start() error {
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				return err
			}
			go s.handle(conn)
		}
	}
}

func (s *TCPServer) Addr() net.Addr {
	return s.listener.Addr()
}

func (s *TCPServer) Close() error {
	s.connMgr.CloseAllConnections()
	return s.listener.Close()
}

func (s *TCPServer) handle(conn net.Conn) {
	defer conn.Close()
	dst, err := s.redirect(conn)
	if err != nil {
		// TODO: log error
		return
	}
	defer s.connMgr.DeleteDestConnection(conn)
	pipe := ioutils.NewBidirectionalPipe(s.ctx, conn, dst)
	pipe.Start()
}

func (s *TCPServer) redirect(conn net.Conn) (net.Conn, error) {
	// Read the stream header once as a handshake.
	var headerBuf [headerSize]byte
	if _, err := io.ReadFull(conn, headerBuf[:]); err != nil {
		return nil, err
	}

	header := ToHeader(headerBuf)
	if !header.Validate() {
		return nil, ErrInvalidHeader
	}

	// get destination connection
	host, port := header.GetHostPort()
	return s.connMgr.GetOrCreateDestConnection(conn, host, port)
}

func (s *TCPServer) createDestConnection(host, port string) (net.Conn, error) {
	addr := host + ":" + port
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
