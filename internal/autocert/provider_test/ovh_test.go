package provider_test

import (
	"testing"

	"github.com/go-acme/lego/v4/providers/dns/ovh"
	U "github.com/yusing/go-proxy/internal/utils"
	. "github.com/yusing/go-proxy/internal/utils/testing"
	"gopkg.in/yaml.v3"
)

// type Config struct {
// 	APIEndpoint string

// 	ApplicationKey    string
// 	ApplicationSecret string
// 	ConsumerKey       string

// 	OAuth2Config *OAuth2Config

// 	PropagationTimeout time.Duration
// 	PollingInterval    time.Duration
// 	TTL                int
// 	HTTPClient         *http.Client
// }

func TestOVH(t *testing.T) {
	cfg := &ovh.Config{}
	testYaml := `
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
	testYaml = testYaml[1:] // remove first \n
	opt := make(map[string]any)
	ExpectNoError(t, yaml.Unmarshal([]byte(testYaml), opt))
	ExpectNoError(t, U.Deserialize(opt, cfg))
	ExpectEqual(t, cfg, cfgExpected)
}
