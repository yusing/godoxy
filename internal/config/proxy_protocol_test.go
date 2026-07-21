package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/goutils/server"
)

func TestEntrypointTrustedProxyConfigValidation(t *testing.T) {
	t.Run("valid IP and CIDR", func(t *testing.T) {
		err := configtypes.Validate([]byte(`
entrypoint:
  proxy_protocol:
    mode: mixed
    trusted_proxies:
      - 127.0.0.1
      - 10.0.0.0/8
      - 2001:db8::/32
`))
		require.NoError(t, err)
	})

	t.Run("invalid entry", func(t *testing.T) {
		err := configtypes.Validate([]byte(`
entrypoint:
  proxy_protocol:
    mode: required
    trusted_proxies:
      - not-an-ip-or-cidr
`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "not-an-ip-or-cidr")
	})

	for _, test := range []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "enabled mode requires trusted proxy",
			yaml: "entrypoint:\n  proxy_protocol:\n    mode: required\n",
			want: "requires at least one trusted proxy",
		},
		{
			name: "unknown future mode",
			yaml: "entrypoint:\n  proxy_protocol:\n    mode: opportunistic\n    trusted_proxies: [127.0.0.1]\n",
			want: "unknown proxy protocol mode",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := configtypes.Validate([]byte(test.yaml))
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
		})
	}

	t.Run("disabled mode ignores trusted proxies", func(t *testing.T) {
		err := configtypes.Validate([]byte("entrypoint:\n  proxy_protocol:\n    mode: disabled\n    trusted_proxies: [127.0.0.1]\n"))
		require.NoError(t, err)
	})
}

func TestSupportProxyProtocolDeprecationWarning(t *testing.T) {
	tests := []struct {
		name        string
		config      entrypoint.Config
		wantWarning string
	}{
		{
			name:        "legacy mode requests trusted proxies",
			config:      entrypoint.Config{SupportProxyProtocol: true},
			wantWarning: "configure entrypoint.proxy_protocol with mode required or mixed and at least one trusted_proxies entry",
		},
		{
			name: "legacy flag with durable config can be removed",
			config: entrypoint.Config{
				SupportProxyProtocol: true,
				ProxyProtocol: &server.ProxyProtocolConfig{
					Mode:           server.ProxyProtocolModeRequired,
					TrustedProxies: []string{"127.0.0.1"},
				},
			},
			wantWarning: "ignored because entrypoint.proxy_protocol is configured",
		},
		{
			name: "durable config has no deprecation warning",
			config: entrypoint.Config{ProxyProtocol: &server.ProxyProtocolConfig{
				Mode:           server.ProxyProtocolModeMixed,
				TrustedProxies: []string{"127.0.0.1"},
			}},
			wantWarning: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warning := proxyProtocolDeprecationWarning(tt.config)
			if tt.wantWarning == "" {
				require.Empty(t, warning)
				return
			}
			require.Contains(t, warning, "support_proxy_protocol")
			require.Contains(t, warning, tt.wantWarning)
		})
	}
}
