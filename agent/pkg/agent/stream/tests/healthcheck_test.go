package stream_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent/stream"
)

func TestTCPHealthCheck(t *testing.T) {
	certs := genTestCerts(t)

	srv := startTCPServer(t, certs)

	err := stream.TCPHealthCheck(t.Context(), srv.Addr.String(), certs.CaCert, certs.ClientCert)
	require.NoError(t, err, "health check")
}

func TestUDPHealthCheck(t *testing.T) {
	certs := genTestCerts(t)

	srv := startUDPServer(t, certs)

	err := stream.UDPHealthCheck(t.Context(), srv.Addr.String(), certs.CaCert, certs.ClientCert)
	require.NoError(t, err, "health check")
}

func TestUDPHealthCheckHonorsContextDeadline(t *testing.T) {
	certs := genTestCerts(t)

	addr, closeFn := startSilentUDPServer(t)
	t.Cleanup(closeFn)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := stream.UDPHealthCheck(ctx, addr, certs.CaCert, certs.ClientCert)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
}

func startSilentUDPServer(t *testing.T) (string, func()) {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)

	return conn.LocalAddr().String(), func() {
		require.NoError(t, conn.Close())
	}
}
