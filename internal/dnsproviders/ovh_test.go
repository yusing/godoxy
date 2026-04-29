package dnsproviders_test

import (
	"testing"

	"github.com/go-acme/lego/v4/providers/dns/ovh"
	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/serialization"
)

func TestOVHConfigDecode(t *testing.T) {
	cfg := &ovh.Config{}
	testYAML := `
api_endpoint: https://eu.api.ovh.com
application_key: <application_key>
application_secret: <application_secret>
consumer_key: <consumer_key>
oauth2_config:
  client_id: <client_id>
  client_secret: <client_secret>
`
	cfgExpected := &ovh.Config{
		APIEndpoint:       "https://eu.api.ovh.com",
		ApplicationKey:    "<application_key>",
		ApplicationSecret: "<application_secret>",
		ConsumerKey:       "<consumer_key>",
		OAuth2Config:      &ovh.OAuth2Config{ClientID: "<client_id>", ClientSecret: "<client_secret>"},
	}

	testYAML = testYAML[1:]
	opt := make(map[string]any)
	require.NoError(t, yaml.Unmarshal([]byte(testYAML), &opt))
	require.NoError(t, serialization.MapUnmarshalValidate(opt, cfg))
	require.Equal(t, cfgExpected, cfg)
}
