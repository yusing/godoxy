package netutils

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsWildcardListenHost(t *testing.T) {
	for _, host := range []string{
		"",
		"0.0.0.0",
		net.JoinHostPort("0.0.0.0", "443"),
		"::",
		"[::]",
		net.JoinHostPort("::", "443"),
		net.JoinHostPort("0:0:0:0:0:0:0:0", "443"),
	} {
		require.True(t, IsWildcardListenHost(host), host)
	}

	for _, host := range []string{
		"127.0.0.1",
		net.JoinHostPort("127.0.0.1", "443"),
		"localhost",
	} {
		require.False(t, IsWildcardListenHost(host), host)
	}
}
