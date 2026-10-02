package env

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddressURLAuthorities(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080", "[2001:db8::1]:8080", ":8080"} {
		t.Run(address, func(t *testing.T) {
			t.Setenv("GODOXY_API_ADDR", address)
			addr, host, port, fullURL := Address("API_ADDR", "http")
			require.Equal(t, address, addr)
			require.Equal(t, "http://"+address, fullURL)
			parsed, err := url.Parse(fullURL)
			require.NoError(t, err)
			require.Equal(t, host, parsed.Hostname())
			require.Equal(t, "8080", parsed.Port())
			require.Equal(t, 8080, port)
		})
	}
}
