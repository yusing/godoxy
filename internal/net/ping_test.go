package netutils

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestPing(t *testing.T) {
	t.Run("localhost", func(t *testing.T) {
		ok, err := Ping(context.Background(), net.ParseIP("127.0.0.1"))
		// ping (ICMP) is not allowed for non-root users
		if errors.Is(err, os.ErrPermission) {
			t.Skip("permission denied")
		}
		expect.NoError(t, err)
		expect.True(t, ok)
	})
}
