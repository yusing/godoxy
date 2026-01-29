package stream_test

import (
	"testing"

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
