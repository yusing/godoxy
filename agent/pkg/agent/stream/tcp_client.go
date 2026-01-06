package stream

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"time"

	"github.com/yusing/godoxy/agent/pkg/agent/common"
)

type TCPClient struct {
	conn net.Conn
}

// NewTCPClient creates a new TCP client for the agent.
//
// It will establish a TLS connection and send a stream request header to the server.
//
// It returns an error if
//   - the target address is invalid
//   - the stream request header is invalid
//   - the TLS configuration is invalid
//   - the TLS connection fails
//   - the stream request header is not sent
func NewTCPClient(serverAddr, targetAddress string, caCert *x509.Certificate, clientCert *tls.Certificate) (net.Conn, error) {
	host, port, err := net.SplitHostPort(targetAddress)
	if err != nil {
		return nil, err
	}

	header, err := NewStreamRequestHeader(host, port)
	if err != nil {
		return nil, err
	}

	// Setup TLS configuration
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*clientCert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
		ServerName:   common.CertsDNSName,
	}

	// Establish TLS connection
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: dialTimeout}, "tcp", serverAddr, tlsConfig)
	if err != nil {
		return nil, err
	}
	// Send the stream header once as a handshake.
	if _, err := conn.Write(header.Bytes()); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &TCPClient{
		conn: conn,
	}, nil
}

func (c *TCPClient) Read(p []byte) (n int, err error) {
	return c.conn.Read(p)
}

func (c *TCPClient) Write(p []byte) (n int, err error) {
	return c.conn.Write(p)
}

func (c *TCPClient) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *TCPClient) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *TCPClient) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *TCPClient) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *TCPClient) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c *TCPClient) Close() error {
	return c.conn.Close()
}
