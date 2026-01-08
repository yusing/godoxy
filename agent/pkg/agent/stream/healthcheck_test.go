package stream_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/pion/transport/v3/udp"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/agent/stream"
)

func TestTCPHealthCheck(t *testing.T) {
	caPEM, srvPEM, clientPEM, err := agent.NewAgent()
	require.NoError(t, err, "generate agent certs")

	caCert, err := caPEM.ToTLSCert()
	require.NoError(t, err, "parse CA cert")
	srvCert, err := srvPEM.ToTLSCert()
	require.NoError(t, err, "parse server cert")
	clientCert, err := clientPEM.ToTLSCert()
	require.NoError(t, err, "parse client cert")

	tcpLn, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err, "listen tcp")
	defer tcpLn.Close()

	srv := stream.NewTCPServer(t.Context(), tcpLn, caCert.Leaf, srvCert)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer func() {
		_ = srv.Close()
		err := <-errCh
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, udp.ErrClosedListener) {
			t.Logf("udp server exit: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	err = stream.TCPHealthCheck(srv.Addr().String(), caCert.Leaf, clientCert)
	require.NoError(t, err, "health check")
}

func TestUDPHealthCheck(t *testing.T) {
	caPEM, srvPEM, clientPEM, err := agent.NewAgent()
	require.NoError(t, err, "generate agent certs")

	caCert, err := caPEM.ToTLSCert()
	require.NoError(t, err, "parse CA cert")
	srvCert, err := srvPEM.ToTLSCert()
	require.NoError(t, err, "parse server cert")
	clientCert, err := clientPEM.ToTLSCert()
	require.NoError(t, err, "parse client cert")

	srv := stream.NewUDPServer(t.Context(), "udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}, caCert.Leaf, srvCert)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer func() {
		_ = srv.Close()
		err := <-errCh
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, udp.ErrClosedListener) {
			t.Logf("udp server exit: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	err = stream.UDPHealthCheck(srv.Addr().String(), caCert.Leaf, clientCert)
	require.NoError(t, err, "health check")
}
