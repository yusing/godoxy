package stream

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"time"

	"github.com/pion/dtls/v3"
	"github.com/yusing/godoxy/agent/pkg/agent/common"
)

type UDPClient struct {
	conn net.Conn
}

// NewUDPClient creates a new UDP client for the agent.
//
// It will establish a DTLS connection and send a stream request header to the server.
//
// It returns an error if
//   - the target address is invalid
//   - the stream request header is invalid
//   - the DTLS configuration is invalid
//   - the DTLS connection fails
//   - the stream request header is not sent
func NewUDPClient(serverAddr, targetAddress string, caCert *x509.Certificate, clientCert *tls.Certificate) (net.Conn, error) {
	host, port, err := net.SplitHostPort(targetAddress)
	if err != nil {
		return nil, err
	}

	header, err := NewStreamRequestHeader(host, port)
	if err != nil {
		return nil, err
	}

	// Setup DTLS configuration
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(caCert)

	dtlsConfig := &dtls.Config{
		Certificates:         []tls.Certificate{*clientCert},
		RootCAs:              caCertPool,
		InsecureSkipVerify:   false,
		ExtendedMasterSecret: dtls.RequireExtendedMasterSecret,
		ServerName:           common.CertsDNSName,
		CipherSuites:         dTLSCipherSuites,
	}

	raddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, err
	}

	// Establish DTLS connection
	conn, err := dtls.Dial("udp", raddr, dtlsConfig)
	if err != nil {
		return nil, err
	}
	// Send the stream header once as a handshake.
	if _, err := conn.Write(header.Bytes()); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &UDPClient{
		conn: conn,
	}, nil
}

func (c *UDPClient) Read(p []byte) (n int, err error) {
	return c.conn.Read(p)
}

func (c *UDPClient) Write(p []byte) (n int, err error) {
	return c.conn.Write(p)
}

func (c *UDPClient) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *UDPClient) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *UDPClient) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *UDPClient) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *UDPClient) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c *UDPClient) Close() error {
	return c.conn.Close()
}
