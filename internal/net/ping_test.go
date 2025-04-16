package netutils

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPing(t *testing.T) {
	t.Run("localhost", func(t *testing.T) {
		ok, err := Ping(context.Background(), net.ParseIP("127.0.0.1"))
		// ping (ICMP) is not allowed for non-root users
		if errors.Is(err, os.ErrPermission) {
			t.Skip("permission denied")
		}
		require.NoError(t, err)
		require.True(t, ok)
	})
}
