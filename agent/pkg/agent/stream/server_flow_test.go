package stream_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pion/dtls/v3"
	"github.com/pion/transport/v3/udp"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/agent/stream"
)

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

func TestTCPServer_FullFlow(t *testing.T) {
	caPEM, srvPEM, clientPEM, err := agent.NewAgent()
	require.NoError(t, err, "generate agent certs")

	caCert, err := caPEM.ToTLSCert()
	require.NoError(t, err, "parse CA cert")
	srvCert, err := srvPEM.ToTLSCert()
	require.NoError(t, err, "parse server cert")
	clientCert, err := clientPEM.ToTLSCert()
	require.NoError(t, err, "parse client cert")

	dstAddr, closeDst := startTCPEcho(t)
	defer closeDst()

	tcpLn, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err, "listen tcp")
	defer tcpLn.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	srv := stream.NewTCPServer(ctx, tcpLn, caCert.Leaf, srvCert)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer func() {
		cancel()
		_ = srv.Close()
		_ = <-errCh
	}()

	client, err := stream.NewTCPClient(srv.Addr().String(), dstAddr, caCert.Leaf, clientCert)
	require.NoError(t, err, "create tcp client")
	defer client.Close()

	// Ensure ALPN is negotiated as expected (required for multiplexing).
	withState, ok := client.(interface{ ConnectionState() tls.ConnectionState })
	require.True(t, ok, "tcp client should expose TLS connection state")
	require.Equal(t, stream.StreamALPN, withState.ConnectionState().NegotiatedProtocol)

	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	msg := []byte("ping over tcp")
	_, err = client.Write(msg)
	require.NoError(t, err, "write to client")

	buf := make([]byte, len(msg))
	_, err = io.ReadFull(client, buf)
	require.NoError(t, err, "read from client")
	require.Equal(t, string(msg), string(buf), "unexpected echo")
}

func TestTCPServer_ConcurrentConnections(t *testing.T) {
	caPEM, srvPEM, clientPEM, err := agent.NewAgent()
	require.NoError(t, err, "generate agent certs")

	caCert, err := caPEM.ToTLSCert()
	require.NoError(t, err, "parse CA cert")
	srvCert, err := srvPEM.ToTLSCert()
	require.NoError(t, err, "parse server cert")
	clientCert, err := clientPEM.ToTLSCert()
	require.NoError(t, err, "parse client cert")

	dstAddr, closeDst := startTCPEcho(t)
	defer closeDst()

	tcpLn, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err, "listen tcp")
	defer tcpLn.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	srv := stream.NewTCPServer(ctx, tcpLn, caCert.Leaf, srvCert)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer func() {
		cancel()
		_ = srv.Close()
		_ = <-errCh
	}()

	const nClients = 25

	errs := make(chan error, nClients)
	var wg sync.WaitGroup
	wg.Add(nClients)

	for i := range nClients {
		go func() {
			defer wg.Done()

			client, err := stream.NewTCPClient(srv.Addr().String(), dstAddr, caCert.Leaf, clientCert)
			if err != nil {
				errs <- fmt.Errorf("create tcp client: %w", err)
				return
			}
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
	caPEM, srvPEM, _, err := agent.NewAgent()
	require.NoError(t, err, "generate agent certs")

	caCert, err := caPEM.ToTLSCert()
	require.NoError(t, err, "parse CA cert")
	srvCert, err := srvPEM.ToTLSCert()
	require.NoError(t, err, "parse server cert")

	// Generate a self-signed client cert that is NOT signed by the CA
	_, _, invalidClientPEM, err := agent.NewAgent()
	require.NoError(t, err, "generate invalid client certs")
	invalidClientCert, err := invalidClientPEM.ToTLSCert()
	require.NoError(t, err, "parse invalid client cert")

	dstAddr, closeDst := startUDPEcho(t)
	defer closeDst()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	srv := stream.NewUDPServer(ctx, "udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}, caCert.Leaf, srvCert)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer func() {
		cancel()
		_ = srv.Close()
		_ = <-errCh
	}()

	time.Sleep(100 * time.Millisecond)

	// Try to connect with a client cert from a different CA
	_, err = stream.NewUDPClient(srv.Addr().String(), dstAddr, caCert.Leaf, invalidClientCert)
	require.Error(t, err, "expected error when connecting with client cert from different CA")

	var handshakeErr *dtls.HandshakeError
	require.ErrorAs(t, err, &handshakeErr, "expected handshake error")
}

func TestUDPServer_RejectClientWithoutCert(t *testing.T) {
	caPEM, srvPEM, _, err := agent.NewAgent()
	require.NoError(t, err, "generate agent certs")

	caCert, err := caPEM.ToTLSCert()
	require.NoError(t, err, "parse CA cert")
	srvCert, err := srvPEM.ToTLSCert()
	require.NoError(t, err, "parse server cert")

	dstAddr, closeDst := startUDPEcho(t)
	defer closeDst()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	srv := stream.NewUDPServer(ctx, "udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}, caCert.Leaf, srvCert)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer func() {
		cancel()
		_ = srv.Close()
		_ = <-errCh
	}()

	time.Sleep(time.Second)

	// Try to connect without any client certificate
	// Create a TLS cert without a private key to simulate no client cert
	emptyCert := &tls.Certificate{}
	_, err = stream.NewUDPClient(srv.Addr().String(), dstAddr, caCert.Leaf, emptyCert)
	require.Error(t, err, "expected error when connecting without client cert")

	require.ErrorContains(t, err, "no certificate provided", "expected no cert error")
}

func TestUDPServer_FullFlow(t *testing.T) {
	caPEM, srvPEM, clientPEM, err := agent.NewAgent()
	require.NoError(t, err, "generate agent certs")

	caCert, err := caPEM.ToTLSCert()
	require.NoError(t, err, "parse CA cert")
	srvCert, err := srvPEM.ToTLSCert()
	require.NoError(t, err, "parse server cert")
	clientCert, err := clientPEM.ToTLSCert()
	require.NoError(t, err, "parse client cert")

	dstAddr, closeDst := startUDPEcho(t)
	defer closeDst()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	srv := stream.NewUDPServer(ctx, "udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}, caCert.Leaf, srvCert)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer func() {
		cancel()
		_ = srv.Close()
		err := <-errCh
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, udp.ErrClosedListener) {
			t.Logf("udp server exit: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	client, err := stream.NewUDPClient(srv.Addr().String(), dstAddr, caCert.Leaf, clientCert)
	require.NoError(t, err, "create udp client")
	defer client.Close()

	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	msg := []byte("ping over udp")
	_, err = client.Write(msg)
	require.NoError(t, err, "write to client")

	buf := make([]byte, 2048)
	n, err := client.Read(buf)
	require.NoError(t, err, "read from client")
	require.Equal(t, string(msg), string(buf[:n]), "unexpected echo")
}

func TestUDPServer_ConcurrentConnections(t *testing.T) {
	caPEM, srvPEM, clientPEM, err := agent.NewAgent()
	require.NoError(t, err, "generate agent certs")

	caCert, err := caPEM.ToTLSCert()
	require.NoError(t, err, "parse CA cert")
	srvCert, err := srvPEM.ToTLSCert()
	require.NoError(t, err, "parse server cert")
	clientCert, err := clientPEM.ToTLSCert()
	require.NoError(t, err, "parse client cert")

	dstAddr, closeDst := startUDPEcho(t)
	defer closeDst()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	srv := stream.NewUDPServer(ctx, "udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}, caCert.Leaf, srvCert)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer func() {
		cancel()
		_ = srv.Close()
		err := <-errCh
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, udp.ErrClosedListener) {
			t.Logf("udp server exit: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	const nClients = 25

	errs := make(chan error, nClients)
	var wg sync.WaitGroup
	wg.Add(nClients)

	for i := range nClients {
		go func() {
			defer wg.Done()

			client, err := stream.NewUDPClient(srv.Addr().String(), dstAddr, caCert.Leaf, clientCert)
			if err != nil {
				errs <- fmt.Errorf("create udp client: %w", err)
				return
			}
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
