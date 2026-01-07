package stream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"time"

	"github.com/pion/dtls/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type UDPServer struct {
	ctx      context.Context
	laddr    *net.UDPAddr
	listener net.Listener

	dtlsConfig *dtls.Config
	connMgr    *ConnectionManager[*net.UDPConn]
}

func NewUDPServer(ctx context.Context, laddr *net.UDPAddr, caCert *x509.Certificate, serverCert *tls.Certificate) *UDPServer {
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(caCert)

	dtlsConfig := &dtls.Config{
		Certificates:         []tls.Certificate{*serverCert},
		ClientCAs:            caCertPool,
		ClientAuth:           dtls.RequireAndVerifyClientCert,
		ExtendedMasterSecret: dtls.RequireExtendedMasterSecret,
	}

	s := &UDPServer{
		ctx:        ctx,
		laddr:      laddr,
		dtlsConfig: dtlsConfig,
	}
	s.connMgr = NewConnectionManager(s.createDestConnection)
	return s
}

func (s *UDPServer) Start() error {
	listener, err := dtls.Listen("udp", s.laddr, s.dtlsConfig)
	if err != nil {
		return err
	}
	s.listener = listener

	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				return err
			}
			go s.handleDTLSConnection(conn)
		}
	}
}

func (s *UDPServer) Addr() net.Addr {
	if s.listener != nil {
		return s.listener.Addr()
	}
	return s.laddr
}

func (s *UDPServer) Close() error {
	s.connMgr.CloseAllConnections()
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
	dstConn, err := s.connMgr.GetOrCreateDestConnection(clientConn, host, port)
	if err != nil {
		s.logger(clientConn).Err(err).Msg("failed to get or create destination connection")
		return
	}
	defer s.connMgr.DeleteDestConnection(clientConn)

	go s.forwardFromDestination(dstConn, clientConn)

	buf := sizedPool.GetSized(65535)
	defer sizedPool.Put(buf)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			n, err := clientConn.Read(buf)
			if err != nil {
				s.logger(clientConn).Err(err).Msg("failed to read from client")
				return
			}
			if _, err := dstConn.Write(buf[:n]); err != nil {
				s.logger(clientConn).Err(err).Msgf("failed to write %d bytes to destination", n)
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
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					return
				}
				s.logger(dstConn).Err(err).Msg("failed to read from destination")
				return
			}
			if _, err := clientConn.Write(buffer[:n]); err != nil {
				s.logger(dstConn).Err(err).Msgf("failed to write %d bytes to client", n)
				return
			}
		}
	}
}
