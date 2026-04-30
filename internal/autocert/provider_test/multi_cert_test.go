//nolint:errchkjson,errcheck
package provider_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/godoxy/internal/serialization"
	"github.com/yusing/goutils/task"
)

func buildMultiCertYAML(serverURL string) []byte {
	return fmt.Appendf(nil, `
email: main@example.com
domains: [main.example.com]
provider: custom
ca_dir_url: %s/acme/acme/directory
cert_path: certs/main.crt
key_path: certs/main.key
extra:
  - email: extra1@example.com
    domains: [extra1.example.com]
    cert_path: certs/extra1.crt
    key_path: certs/extra1.key
  - email: extra2@example.com
    domains: [extra2.example.com]
    cert_path: certs/extra2.crt
    key_path: certs/extra2.key
`, serverURL)
}

func TestMultipleCertificatesLifecycle(t *testing.T) {
	acmeServer := newTestACMEServer(t)
	defer acmeServer.Close()

	yamlConfig := buildMultiCertYAML(acmeServer.URL())
	var cfg autocert.Config
	cfg.HTTPClient = acmeServer.httpClient()

	/* unmarshal yaml config with multiple certs */
	err := error(serialization.UnmarshalValidate(yamlConfig, &cfg, yaml.Unmarshal))
	require.NoError(t, err)
	require.Equal(t, []string{"main.example.com"}, cfg.Domains)
	require.Len(t, cfg.Extra, 2)
	require.Equal(t, []string{"extra1.example.com"}, cfg.Extra[0].Domains)
	require.Equal(t, []string{"extra2.example.com"}, cfg.Extra[1].Domains)

	var provider *autocert.Provider

	/* initialize autocert with multi-cert config */
	user, legoCfg, gerr := cfg.GetLegoConfig()
	require.NoError(t, gerr)
	provider, err = autocert.NewProvider(&cfg, user, legoCfg)
	require.NoError(t, err)
	require.NotNil(t, provider)

	// Start renewal scheduler
	root := task.RootTask("test", false)
	defer root.Finish(nil)
	provider.ScheduleRenewalAll(root)

	require.Equal(t, "custom", cfg.Provider)
	require.Equal(t, "custom", cfg.Extra[0].Provider)
	require.Equal(t, "custom", cfg.Extra[1].Provider)

	/* track cert requests for all configs */
	os.MkdirAll("certs", 0o755)
	defer os.RemoveAll("certs")

	err = provider.ObtainCertIfNotExistsAll()
	require.NoError(t, err)

	require.Equal(t, 1, acmeServer.certRequestCount["main.example.com"])
	require.Equal(t, 1, acmeServer.certRequestCount["extra1.example.com"])
	require.Equal(t, 1, acmeServer.certRequestCount["extra2.example.com"])

	/* track renewal scheduling and requests */

	// force renewal for all providers and wait for completion
	ok := provider.ForceExpiryAll()
	require.True(t, ok)
	provider.WaitRenewalDone(t.Context())

	require.Equal(t, 1, acmeServer.renewalRequestCount["main.example.com"])
	require.Equal(t, 1, acmeServer.renewalRequestCount["extra1.example.com"])
	require.Equal(t, 1, acmeServer.renewalRequestCount["extra2.example.com"])
}
