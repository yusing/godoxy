package autocert_test

import (
	"encoding/json/v2"
	"testing"
	"time"

	"github.com/go-acme/lego/v5/providers/dns/acmedns"
	"github.com/go-acme/lego/v5/providers/dns/azuredns"
	"github.com/go-acme/lego/v5/providers/dns/httpreq"
	"github.com/go-acme/lego/v5/providers/dns/ovh"
	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/godoxy/internal/dnsproviders"
	"github.com/yusing/godoxy/internal/serialization"
)

func captureProviderConfig[T any](t *testing.T, options autocert.ProviderOptions) *T {
	t.Helper()
	var captured *T
	factory := autocert.DNSProvider(func() *T { return new(T) }, func(cfg *T) (*dnsproviders.DummyProvider, error) {
		captured = cfg
		return &dnsproviders.DummyProvider{}, nil
	})
	require.NoError(t, func() error { _, err := factory.New(options); return err }())
	require.NotNil(t, captured)
	return captured
}

func TestProviderOptionsNestedFieldsAndScalarConversions(t *testing.T) {
	var options autocert.ProviderOptions
	require.NoError(t, serialization.MapUnmarshalValidate(serialization.SerializedObject{
		"oauth2_config":       serialization.SerializedObject{"client_id": "id", "client_secret": "secret"},
		"ttl":                 120,
		"propagation_timeout": "1m30s",
	}, &options))
	cfg := captureProviderConfig[ovh.Config](t, options)
	require.Equal(t, &ovh.OAuth2Config{ClientID: "id", ClientSecret: "secret"}, cfg.OAuth2Config)
	require.Equal(t, 120, cfg.TTL)
	require.Equal(t, 90*time.Second, cfg.PropagationTimeout)

	require.NoError(t, json.Unmarshal([]byte(`{"private_zone":true,"ttl":"60","environment":{"active_directory_authority_host":"https://login.example.com","services":{"resourceManager":{"audience":"audience","endpoint":"https://api.example.com"}}}}`), &options))
	azure := captureProviderConfig[azuredns.Config](t, options)
	require.True(t, azure.PrivateZone)
	require.Equal(t, 60, azure.TTL)
	require.Equal(t, "https://login.example.com", azure.Environment.ActiveDirectoryAuthorityHost)
	require.Equal(t, "https://api.example.com", azure.Environment.Services["resourceManager"].Endpoint)
}

func TestProviderOptionsArraysAndEncodedLegacyValues(t *testing.T) {
	for _, input := range []string{
		`{"allow_list":["192.0.2.1","198.51.100.1"]}`,
		`{"allow_list":"192.0.2.1,198.51.100.1"}`,
	} {
		var options autocert.ProviderOptions
		require.NoError(t, json.Unmarshal([]byte(input), &options))
		cfg := captureProviderConfig[acmedns.Config](t, options)
		require.Equal(t, []string{"192.0.2.1", "198.51.100.1"}, cfg.AllowList)
	}
	var options autocert.ProviderOptions
	require.NoError(t, json.Unmarshal([]byte(`{"oauth2_config":"{client_id: id, client_secret: secret}"}`), &options))
	cfg := captureProviderConfig[ovh.Config](t, options)
	require.Equal(t, "secret", cfg.OAuth2Config.ClientSecret)
}

func TestProviderOptionsYAMLReachesConcreteNestedConfig(t *testing.T) {
	dnsproviders.InitProviders()
	var cfg autocert.Config
	require.NoError(t, serialization.UnmarshalValidate([]byte(`
provider: pseudo
options:
  endpoint:
    scheme: https
    host: challenge.example.com
    path: /dns
  polling_interval: 500ms
`), &cfg, yaml.Unmarshal))
	httpConfig := captureProviderConfig[httpreq.Config](t, cfg.Options)
	require.Equal(t, "https://challenge.example.com/dns", httpConfig.Endpoint.String())
	require.Equal(t, 500*time.Millisecond, httpConfig.PollingInterval)
}

func TestProviderOptionsYAMLInputAndRedaction(t *testing.T) {
	const secret = "nested-provider-credential-sentinel"
	var cfg autocert.Config
	require.NoError(t, serialization.MapUnmarshalValidate(serialization.SerializedObject{
		"options": serialization.SerializedObject{"oauth2_config": serialization.SerializedObject{"client_secret": secret}},
	}, &cfg))
	for _, marshal := range []func(any) ([]byte, error){
		func(v any) ([]byte, error) { return json.Marshal(v) },
		yaml.Marshal,
	} {
		data, err := marshal(cfg.Options)
		require.NoError(t, err)
		require.NotContains(t, string(data), secret)
	}
	var options autocert.ProviderOptions
	require.NoError(t, serialization.MapUnmarshalValidate(serialization.SerializedObject{
		"ttl": 0, "private_zone": false,
	}, &options))
	azure := captureProviderConfig[azuredns.Config](t, options)
	require.Zero(t, azure.TTL)
	require.False(t, azure.PrivateZone)
}
