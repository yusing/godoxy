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
		h := host
		t.Run(h, func(t *testing.T) {
			require.True(t, IsWildcardListenHost(h), h)
		})
	}

	for _, host := range []string{
		"127.0.0.1",
		net.JoinHostPort("127.0.0.1", "443"),
		"localhost",
	} {
		h := host
		t.Run(h, func(t *testing.T) {
			require.False(t, IsWildcardListenHost(h), h)
		})
	}
}
