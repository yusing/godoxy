package autocert_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/godoxy/internal/dnsproviders"
	"github.com/yusing/godoxy/internal/serialization"
	strutils "github.com/yusing/goutils/strings"
)

func TestSetupExtraProviders(t *testing.T) {
	dnsproviders.InitProviders()

	cfgYAML := `
email: test@example.com
domains: [example.com]
provider: custom
ca_dir_url: https://ca.example.com:9000/acme/acme/directory
cert_path: certs/test.crt
key_path: certs/test.key
options: {key: value}
resolvers: [8.8.8.8]
ca_certs: [ca.crt]
eab_kid: eabKid
eab_hmac: eabHmac
extra:
  - cert_path: certs/extra.crt
    key_path: certs/extra.key
  - cert_path: certs/extra2.crt
    key_path: certs/extra2.key
    email: override@example.com
    provider: pseudo
    domains: [override.com]
    ca_dir_url: https://ca2.example.com/directory
    options: {opt2: val2}
    resolvers: [1.1.1.1]
    ca_certs: [ca2.crt]
    eab_kid: eabKid2
    eab_hmac: eabHmac2
`

	var cfg autocert.Config
	err := error(serialization.UnmarshalValidateYAML([]byte(cfgYAML), &cfg))
	require.NoError(t, err)

	// Test: extra[0] inherits all fields from main except CertPath and KeyPath.
	merged0 := cfg.Extra[0]
	require.Equal(t, "certs/extra.crt", merged0.CertPath)
	require.Equal(t, "certs/extra.key", merged0.KeyPath)
	// Inherited fields from main config:
	require.Equal(t, "test@example.com", merged0.Email)                             // inherited
	require.Equal(t, "custom", merged0.Provider)                                    // inherited
	require.Equal(t, []string{"example.com"}, merged0.Domains)                      // inherited
	require.Equal(t, "https://ca.example.com:9000/acme/acme/directory", merged0.CADirURL) // inherited
	require.Equal(t, map[string]strutils.Redacted{"key": "value"}, merged0.Options) // inherited
	require.Equal(t, []string{"8.8.8.8"}, merged0.Resolvers)                        // inherited
	require.Equal(t, []string{"ca.crt"}, merged0.CACerts)                           // inherited
	require.Equal(t, "eabKid", merged0.EABKid)                                      // inherited
	require.Equal(t, "eabHmac", merged0.EABHmac)                                    // inherited
	require.Equal(t, cfg.HTTPClient, merged0.HTTPClient)                            // inherited
	require.Nil(t, merged0.Extra)

	// Test: extra[1] overrides some fields, and inherits others.
	merged1 := cfg.Extra[1]
	require.Equal(t, "certs/extra2.crt", merged1.CertPath)
	require.Equal(t, "certs/extra2.key", merged1.KeyPath)
	// Overridden fields:
	require.Equal(t, "override@example.com", merged1.Email)                         // overridden
	require.Equal(t, "pseudo", merged1.Provider)                                    // overridden
	require.Equal(t, []string{"override.com"}, merged1.Domains)                     // overridden
	require.Equal(t, "https://ca2.example.com/directory", merged1.CADirURL)         // overridden
	require.Equal(t, map[string]strutils.Redacted{"opt2": "val2"}, merged1.Options) // overridden
	require.Equal(t, []string{"1.1.1.1"}, merged1.Resolvers)                        // overridden
	require.Equal(t, []string{"ca2.crt"}, merged1.CACerts)                          // overridden
	require.Equal(t, "eabKid2", merged1.EABKid)                                     // overridden
	require.Equal(t, "eabHmac2", merged1.EABHmac)                                   // overridden
	// Inherited field:
	require.Equal(t, cfg.HTTPClient, merged1.HTTPClient) // inherited
	require.Nil(t, merged1.Extra)
}
