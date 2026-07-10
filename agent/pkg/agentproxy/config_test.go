package agentproxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigFromHeaders(t *testing.T) {
	t.Run("legacy when modern headers are absent", func(t *testing.T) {
		h := make(http.Header)
		h.Set(HeaderXProxyHost, "backend:443")
		h.Set(HeaderXProxyHTTPS, "true")

		cfg, err := ConfigFromHeaders(h)
		require.NoError(t, err)
		require.Equal(t, "https", cfg.Scheme)
		require.Equal(t, "backend:443", cfg.Host)
	})

	for _, test := range []struct {
		name string
		set  func(http.Header)
	}{
		{
			name: "malformed modern config",
			set: func(h http.Header) {
				h.Set(HeaderXProxyHost, "backend")
				h.Set(HeaderXProxyScheme, "https")
				h.Set(HeaderXProxyConfig, "not-base64")
			},
		},
		{
			name: "missing modern host",
			set: func(h http.Header) {
				h.Set(HeaderXProxyScheme, "http")
				h.Set(HeaderXProxyConfig, "e30=")
			},
		},
		{
			name: "unsupported modern scheme",
			set: func(h http.Header) {
				h.Set(HeaderXProxyHost, "backend")
				h.Set(HeaderXProxyScheme, "ftp")
				h.Set(HeaderXProxyConfig, "e30=")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := make(http.Header)
			test.set(h)
			_, err := ConfigFromHeaders(h)
			require.Error(t, err)
		})
	}

	for _, scheme := range []string{"http", "https", "h2c"} {
		t.Run(scheme, func(t *testing.T) {
			h := make(http.Header)
			(&Config{Scheme: scheme, Host: "backend"}).SetAgentProxyConfigHeaders(h)
			cfg, err := ConfigFromHeaders(h)
			require.NoError(t, err)
			require.Equal(t, scheme, cfg.Scheme)
		})
	}
}
