package stream_test

import (
	"crypto/tls"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/pion/dtls/v3"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/agent/stream"
)

func TestTCPServer_FullFlow(t *testing.T) {
	certs := genTestCerts(t)

	dstAddr, closeDst := startTCPEcho(t)
	defer closeDst()

	srv := startTCPServer(t, certs)

	client := NewTCPClient(t, srv.Addr.String(), dstAddr, certs)
	defer client.Close()

	// Ensure ALPN is negotiated as expected (required for multiplexing).
	withState, ok := client.(interface{ ConnectionState() tls.ConnectionState })
	require.True(t, ok, "tcp client should expose TLS connection state")
	require.Equal(t, stream.StreamALPN, withState.ConnectionState().NegotiatedProtocol)

	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	msg := []byte("ping over tcp")
	_, err := client.Write(msg)
	require.NoError(t, err, "write to client")

	buf := make([]byte, len(msg))
	_, err = io.ReadFull(client, buf)
	require.NoError(t, err, "read from client")
	require.Equal(t, string(msg), string(buf), "unexpected echo")
}

func TestTCPServer_ConcurrentConnections(t *testing.T) {
	certs := genTestCerts(t)

	dstAddr, closeDst := startTCPEcho(t)
	defer closeDst()

	srv := startTCPServer(t, certs)

	const nClients = 25

	errs := make(chan error, nClients)
	var wg sync.WaitGroup
	wg.Add(nClients)

	for i := range nClients {
		go func() {
			defer wg.Done()

			client := NewTCPClient(t, srv.Addr.String(), dstAddr, certs)
			defer client.Close()

			_ = client.SetDeadline(time.Now().Add(2 * time.Second))
			msg := fmt.Appendf(nil, "ping over tcp %d", i)
			if _, err := client.Write(msg); err != nil {
				errs <- fmt.Errorf("write to client: %w", err)
				return
			}

			buf := make([]byte, len(msg))
			if _, err := io.ReadFull(client, buf); err != nil {
				errs <- fmt.Errorf("read from client: %w", err)
				return
			}
			if string(msg) != string(buf) {
				errs <- fmt.Errorf("unexpected echo: got=%q want=%q", string(buf), string(msg))
				return
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

func TestUDPServer_RejectInvalidClient(t *testing.T) {
	certs := genTestCerts(t)

	// Generate a self-signed client cert that is NOT signed by the CA
	_, _, invalidClientPEM, err := agent.NewAgent()
	require.NoError(t, err, "generate invalid client certs")
	invalidClientCert, err := invalidClientPEM.ToTLSCert()
	require.NoError(t, err, "parse invalid client cert")

	dstAddr, closeDst := startUDPEcho(t)
	defer closeDst()

	srv := startUDPServer(t, certs)

	// Try to connect with a client cert from a different CA
	_, err = stream.NewUDPClient(srv.Addr.String(), dstAddr, certs.CaCert, invalidClientCert)
	require.Error(t, err, "expected error when connecting with client cert from different CA")

	var handshakeErr *dtls.HandshakeError
	require.ErrorAs(t, err, &handshakeErr, "expected handshake error")
}

func TestUDPServer_RejectClientWithoutCert(t *testing.T) {
	certs := genTestCerts(t)

	dstAddr, closeDst := startUDPEcho(t)
	defer closeDst()

	srv := startUDPServer(t, certs)

	time.Sleep(time.Second)

	// Try to connect without any client certificate
	// Create a TLS cert without a private key to simulate no client cert
	emptyCert := &tls.Certificate{}
	_, err := stream.NewUDPClient(srv.Addr.String(), dstAddr, certs.CaCert, emptyCert)
	require.Error(t, err, "expected error when connecting without client cert")

	require.ErrorContains(t, err, "no certificate provided", "expected no cert error")
}

func TestUDPServer_FullFlow(t *testing.T) {
	certs := genTestCerts(t)

	dstAddr, closeDst := startUDPEcho(t)
	defer closeDst()

	srv := startUDPServer(t, certs)

	client := NewUDPClient(t, srv.Addr.String(), dstAddr, certs)
	defer client.Close()

	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	msg := []byte("ping over udp")
	_, err := client.Write(msg)
	require.NoError(t, err, "write to client")

	buf := make([]byte, 2048)
	n, err := client.Read(buf)
	require.NoError(t, err, "read from client")
	require.Equal(t, string(msg), string(buf[:n]), "unexpected echo")
}

func TestUDPServer_ConcurrentConnections(t *testing.T) {
	certs := genTestCerts(t)

	dstAddr, closeDst := startUDPEcho(t)
	defer closeDst()

	srv := startUDPServer(t, certs)

	const nClients = 25

	errs := make(chan error, nClients)
	var wg sync.WaitGroup
	wg.Add(nClients)

	for i := range nClients {
		go func() {
			defer wg.Done()

			client := NewUDPClient(t, srv.Addr.String(), dstAddr, certs)
			defer client.Close()

			_ = client.SetDeadline(time.Now().Add(5 * time.Second))
			msg := fmt.Appendf(nil, "ping over udp %d", i)
			if _, err := client.Write(msg); err != nil {
				errs <- fmt.Errorf("write to client: %w", err)
				return
			}

			buf := make([]byte, 2048)
			n, err := client.Read(buf)
			if err != nil {
				errs <- fmt.Errorf("read from client: %w", err)
				return
			}
			if string(msg) != string(buf[:n]) {
				errs <- fmt.Errorf("unexpected echo: got=%q want=%q", string(buf[:n]), string(msg))
				return
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
