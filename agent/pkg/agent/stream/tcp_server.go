package stream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	ioutils "github.com/yusing/goutils/io"
)

type TCPServer struct {
	ctx      context.Context
	listener net.Listener
}

// NewTCPServerHandler creates a TCP stream server that can serve already-accepted
// connections (e.g. handed off by an ALPN multiplexer).
//
// This variant does not require a listener. Use TCPServer.ServeConn to handle
// each incoming stream connection.
func NewTCPServerHandler(ctx context.Context) *TCPServer {
	s := &TCPServer{ctx: ctx}
	return s
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
	if s.listener == nil {
		return net.ErrClosed
	}
	context.AfterFunc(s.ctx, func() {
		_ = s.listener.Close()
	})
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) && s.ctx.Err() != nil {
				return s.ctx.Err()
			}
			return err
		}
		go s.handle(conn)
	}
}

// ServeConn serves a single stream connection.
//
// The provided connection is expected to be already secured (TLS/mTLS) and to
// speak the stream protocol (i.e. the client will send the stream header first).
//
// This method blocks until the stream finishes.
func (s *TCPServer) ServeConn(conn net.Conn) {
	s.handle(conn)
}

func (s *TCPServer) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

func (s *TCPServer) Close() error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *TCPServer) logger(clientConn net.Conn) *zerolog.Logger {
	ev := log.With().Str("protocol", "tcp").
		Str("remote", clientConn.RemoteAddr().String())
	if s.listener != nil {
		ev = ev.Str("addr", s.listener.Addr().String())
	}
	l := ev.Logger()
	return &l
}

func (s *TCPServer) loggerWithDst(dstConn net.Conn, clientConn net.Conn) *zerolog.Logger {
	ev := log.With().Str("protocol", "tcp").
		Str("remote", clientConn.RemoteAddr().String()).
		Str("dst", dstConn.RemoteAddr().String())
	if s.listener != nil {
		ev = ev.Str("addr", s.listener.Addr().String())
	}
	l := ev.Logger()
	return &l
}

func (s *TCPServer) handle(conn net.Conn) {
	defer conn.Close()
	dst, err := s.redirect(conn)
	if err != nil {
		// Health check probe: close connection
		if errors.Is(err, ErrCloseImmediately) {
			s.logger(conn).Info().Msg("Health check received")
			return
		}
		s.logger(conn).Err(err).Msg("failed to redirect connection")
		return
	}

	defer dst.Close()
	pipe := ioutils.NewBidirectionalPipe(s.ctx, conn, dst)
	err = pipe.Start()
	if err != nil {
		s.loggerWithDst(dst, conn).Err(err).Msg("failed to start bidirectional pipe")
		return
	}
}

func (s *TCPServer) redirect(conn net.Conn) (net.Conn, error) {
	// Read the stream header once as a handshake.
	var headerBuf [headerSize]byte
	if _, err := io.ReadFull(conn, headerBuf[:]); err != nil {
		return nil, err
	}

	header := ToHeader(&headerBuf)
	if !header.Validate() {
		return nil, ErrInvalidHeader
	}

	// Health check: close immediately if FlagCloseImmediately is set
	if header.ShouldCloseImmediately() {
		return nil, ErrCloseImmediately
	}

	// get destination connection
	host, port := header.GetHostPort()
	return s.createDestConnection(host, port)
}

func (s *TCPServer) createDestConnection(host, port string) (net.Conn, error) {
	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
