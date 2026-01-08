package stream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"time"

	"github.com/pion/dtls/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type UDPServer struct {
	ctx      context.Context
	network  string
	laddr    *net.UDPAddr
	listener net.Listener

	dtlsConfig *dtls.Config
}

func NewUDPServer(ctx context.Context, network string, laddr *net.UDPAddr, caCert *x509.Certificate, serverCert *tls.Certificate) *UDPServer {
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(caCert)

	dtlsConfig := &dtls.Config{
		Certificates:         []tls.Certificate{*serverCert},
		ClientCAs:            caCertPool,
		ClientAuth:           dtls.RequireAndVerifyClientCert,
		ExtendedMasterSecret: dtls.RequireExtendedMasterSecret,
		CipherSuites:         dTLSCipherSuites,
	}

	s := &UDPServer{
		ctx:        ctx,
		network:    network,
		laddr:      laddr,
		dtlsConfig: dtlsConfig,
	}
	return s
}

func (s *UDPServer) Start() error {
	listener, err := dtls.Listen(s.network, s.laddr, s.dtlsConfig)
	if err != nil {
		return err
	}
	s.listener = listener

	context.AfterFunc(s.ctx, func() {
		_ = s.listener.Close()
	})

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// Expected error when context cancelled
			if errors.Is(err, net.ErrClosed) && s.ctx.Err() != nil {
				return s.ctx.Err()
			}
			return err
		}
		go s.handleDTLSConnection(conn)
	}
}

func (s *UDPServer) Addr() net.Addr {
	if s.listener != nil {
		return s.listener.Addr()
	}
	return s.laddr
}

func (s *UDPServer) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *UDPServer) logger(clientConn net.Conn) *zerolog.Logger {
	l := log.With().Str("protocol", "udp").
		Str("addr", s.laddr.String()).
		Str("remote", clientConn.RemoteAddr().String()).Logger()
	return &l
}

func (s *UDPServer) loggerWithDst(clientConn net.Conn, dstConn *net.UDPConn) *zerolog.Logger {
	l := log.With().Str("protocol", "udp").
		Str("addr", s.laddr.String()).
		Str("remote", clientConn.RemoteAddr().String()).
		Str("dst", dstConn.RemoteAddr().String()).Logger()
	return &l
}

func (s *UDPServer) handleDTLSConnection(clientConn net.Conn) {
	defer clientConn.Close()

	// Read the stream header once as a handshake.
	var headerBuf [headerSize]byte
	if _, err := io.ReadFull(clientConn, headerBuf[:]); err != nil {
		s.logger(clientConn).Err(err).Msg("failed to read stream header")
		return
	}
	header := ToHeader(headerBuf)
	if !header.Validate() {
		s.logger(clientConn).Error().Bytes("header", headerBuf[:]).Msg("invalid stream header received")
		return
	}

	host, port := header.GetHostPort()
	dstConn, err := s.createDestConnection(host, port)
	if err != nil {
		s.logger(clientConn).Err(err).Msg("failed to get or create destination connection")
		return
	}
	defer dstConn.Close()

	go s.forwardFromDestination(dstConn, clientConn)

	buf := sizedPool.GetSized(65535)
	defer sizedPool.Put(buf)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			n, err := clientConn.Read(buf)
			// Per net.Conn contract, Read may return (n > 0, err == io.EOF).
			// Always forward any bytes we got before acting on the error.
			if n > 0 {
				if _, werr := dstConn.Write(buf[:n]); werr != nil {
					s.logger(clientConn).Err(werr).Msgf("failed to write %d bytes to destination", n)
					return
				}
			}
			if err != nil {
				// Expected shutdown paths.
				if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
					return
				}
				s.logger(clientConn).Err(err).Msg("failed to read from client")
				return
			}
		}
	}
}

func (s *UDPServer) createDestConnection(host, port string) (*net.UDPConn, error) {
	addr := host + ":" + port
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	dstConn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, err
	}

	return dstConn, nil
}

func (s *UDPServer) forwardFromDestination(dstConn *net.UDPConn, clientConn net.Conn) {
	buffer := sizedPool.GetSized(65535)
	defer sizedPool.Put(buffer)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			_ = dstConn.SetReadDeadline(time.Now().Add(readDeadline))
			n, err := dstConn.Read(buffer)
			if err != nil {
				// The destination socket can be closed when the client disconnects (e.g. during
				// the stream support probe in AgentConfig.StartWithCerts). Treat that as a
				// normal exit and avoid noisy logs.
				if errors.Is(err, net.ErrClosed) {
					return
				}
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				s.loggerWithDst(clientConn, dstConn).Err(err).Msg("failed to read from destination")
				return
			}
			if _, err := clientConn.Write(buffer[:n]); err != nil {
				s.loggerWithDst(clientConn, dstConn).Err(err).Msgf("failed to write %d bytes to client", n)
				return
			}
		}
	}
}
