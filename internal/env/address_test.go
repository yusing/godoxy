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

func TestSharedHTTPSAddressPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, want, source string
		values             map[string]string
	}{
		{name: "unset", want: ":443", source: "default"},
		{name: "explicit empty", source: "GODOXY_HTTPS_ADDR", values: map[string]string{"GODOXY_HTTPS_ADDR": ""}},
		{name: "empty overrides aliases", source: "GODOXY_HTTPS_ADDR", values: map[string]string{"GODOXY_HTTPS_ADDR": "", "GOPROXY_HTTPS_ADDR": ":8443", "HTTPS_ADDR": ":9443"}},
		{name: "legacy empty", source: "GOPROXY_HTTPS_ADDR", values: map[string]string{"GOPROXY_HTTPS_ADDR": "", "HTTPS_ADDR": ":9443"}},
		{name: "plain empty", source: "HTTPS_ADDR", values: map[string]string{"HTTPS_ADDR": ""}},
		{name: "configured", want: "[::1]:8443", source: "GODOXY_HTTPS_ADDR", values: map[string]string{"GODOXY_HTTPS_ADDR": "[::1]:8443"}},
		{name: "legacy", want: ":8443", source: "GOPROXY_HTTPS_ADDR", values: map[string]string{"GOPROXY_HTTPS_ADDR": ":8443"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			for key, value := range tc.values {
				t.Setenv(key, value)
			}
			addr, host, port, fullURL := Address("HTTPS_ADDR", "https")
			require.Equal(t, tc.want, addr)
			require.Equal(t, tc.want, String("HTTPS_ADDR"))
			if tc.want == "" {
				require.Empty(t, host)
				require.Zero(t, port)
				require.Empty(t, fullURL)
			}
			for _, setting := range Inspect().Settings {
				if setting.Name == "GODOXY_HTTPS_ADDR" {
					require.Equal(t, tc.want, setting.Value)
					require.Equal(t, tc.source, setting.Source)
					return
				}
			}
			t.Fatal("HTTPS diagnostic missing")
		})
	}
}
