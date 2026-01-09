package stream_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/pion/transport/v3/udp"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/agent/stream"
)

// CertBundle holds all certificates needed for testing.
type CertBundle struct {
	CaCert     *x509.Certificate
	SrvCert    *tls.Certificate
	ClientCert *tls.Certificate
}

// genTestCerts generates certificates for testing and returns them as a CertBundle.
func genTestCerts(t *testing.T) CertBundle {
	t.Helper()

	caPEM, srvPEM, clientPEM, err := agent.NewAgent()
	require.NoError(t, err, "generate agent certs")

	caCert, err := caPEM.ToTLSCert()
	require.NoError(t, err, "parse CA cert")
	srvCert, err := srvPEM.ToTLSCert()
	require.NoError(t, err, "parse server cert")
	clientCert, err := clientPEM.ToTLSCert()
	require.NoError(t, err, "parse client cert")

	return CertBundle{
		CaCert:     caCert.Leaf,
		SrvCert:    srvCert,
		ClientCert: clientCert,
	}
}

// startTCPEcho starts a TCP echo server and returns its address and close function.
func startTCPEcho(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "listen tcp")

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(c)
		}
	}()

	return ln.Addr().String(), func() {
		_ = ln.Close()
		<-done
	}
}

// startUDPEcho starts a UDP echo server and returns its address and close function.
func startUDPEcho(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err, "listen udp")
	uc := pc.(*net.UDPConn)

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 65535)
		for {
			n, raddr, err := uc.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = uc.WriteToUDP(buf[:n], raddr)
		}
	}()

	return uc.LocalAddr().String(), func() {
		_ = uc.Close()
		<-done
	}
}

// TestServer wraps a server with its startup goroutine for cleanup.
type TestServer struct {
	Server interface{ Close() error }
	Addr   net.Addr
}

// startTCPServer starts a TCP server and returns a TestServer for cleanup.
func startTCPServer(t *testing.T, certs CertBundle) TestServer {
	t.Helper()

	tcpLn, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err, "listen tcp")

	ctx, cancel := context.WithCancel(t.Context())

	srv := stream.NewTCPServer(ctx, tcpLn, certs.CaCert, certs.SrvCert)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	t.Cleanup(func() {
		cancel()
		_ = srv.Close()
		err := <-errCh
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
			t.Logf("tcp server exit: %v", err)
		}
	})

	return TestServer{
		Server: srv,
		Addr:   srv.Addr(),
	}
}

// startUDPServer starts a UDP server and returns a TestServer for cleanup.
func startUDPServer(t *testing.T, certs CertBundle) TestServer {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())

	srv := stream.NewUDPServer(ctx, "udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}, certs.CaCert, certs.SrvCert)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	time.Sleep(100 * time.Millisecond)

	t.Cleanup(func() {
		cancel()
		_ = srv.Close()
		err := <-errCh
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) && !errors.Is(err, udp.ErrClosedListener) {
			t.Logf("udp server exit: %v", err)
		}
	})

	return TestServer{
		Server: srv,
		Addr:   srv.Addr(),
	}
}

// NewTCPClient creates a TCP client connected to the server with test certificates.
func NewTCPClient(t *testing.T, serverAddr, targetAddress string, certs CertBundle) net.Conn {
	t.Helper()
	client, err := stream.NewTCPClient(serverAddr, targetAddress, certs.CaCert, certs.ClientCert)
	require.NoError(t, err, "create tcp client")
	return client
}

// NewUDPClient creates a UDP client connected to the server with test certificates.
func NewUDPClient(t *testing.T, serverAddr, targetAddress string, certs CertBundle) net.Conn {
	t.Helper()
	client, err := stream.NewUDPClient(serverAddr, targetAddress, certs.CaCert, certs.ClientCert)
	require.NoError(t, err, "create udp client")
	return client
}
