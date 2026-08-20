package autocert_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/go-acme/lego/v5/certcrypto"
	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/godoxy/internal/dnsproviders"
	"github.com/yusing/godoxy/internal/serialization"
	strutils "github.com/yusing/goutils/strings"
)

func TestEABConfigRequired(t *testing.T) {
	dnsproviders.InitProviders()

	tests := []struct {
		name    string
		cfg     *autocert.Config
		wantErr bool
	}{
		{name: "Missing EABKid", cfg: &autocert.Config{EABHmac: "1234567890"}, wantErr: true},
		{name: "Missing EABHmac", cfg: &autocert.Config{EABKid: "1234567890"}, wantErr: true},
		{name: "Valid EAB", cfg: &autocert.Config{EABKid: "1234567890", EABHmac: "1234567890"}, wantErr: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			yamlCfg := fmt.Appendf(nil, "eab_kid: %s\neab_hmac: %s", test.cfg.EABKid, test.cfg.EABHmac)
			cfg := autocert.Config{}
			err := serialization.UnmarshalValidate(yamlCfg, &cfg, yaml.Unmarshal)
			if (err != nil) != test.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestRegisteredProviderKeys(t *testing.T) {
	dnsproviders.InitProviders()

	// lego v5 renamed the rfc2136 package to dnsupdate; both keys stay valid so
	// existing `provider: rfc2136` configs keep working.
	for _, key := range []string{"rfc2136", "dnsupdate"} {
		t.Run("registered/"+key, func(t *testing.T) {
			require.Contains(t, autocert.Providers, key)
		})
	}

	// lego v5 dropped googledomains after Google shut the service down.
	t.Run("googledomains removed", func(t *testing.T) {
		require.NotContains(t, autocert.Providers, "googledomains")
	})
}

func TestEABHmacSerializationRedacted(t *testing.T) {
	const secret = "eab-hmac-credential-sentinel"
	cfg := autocert.Config{EABHmac: secret}

	jsonRepr, err := json.Marshal(cfg)
	require.NoError(t, err)
	yamlRepr, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	redacted := strutils.Redact(secret)
	for format, repr := range map[string][]byte{
		"JSON": jsonRepr,
		"YAML": yamlRepr,
	} {
		t.Run(format, func(t *testing.T) {
			serialized := string(repr)
			require.NotContains(t, serialized, secret)
			require.True(t, strings.Contains(serialized, redacted), "serialized value should contain the redacted credential: %s", serialized)
		})
	}
}

func TestExtraCertKeyPathsUnique(t *testing.T) {
	t.Run("duplicate cert_path rejected", func(t *testing.T) {
		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			Extra: []autocert.ConfigExtra{
				{CertPath: "a.crt", KeyPath: "a.key"},
				{CertPath: "a.crt", KeyPath: "b.key"},
			},
		}
		require.Error(t, cfg.Validate())
	})

	t.Run("duplicate key_path rejected", func(t *testing.T) {
		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			Extra: []autocert.ConfigExtra{
				{CertPath: "a.crt", KeyPath: "a.key"},
				{CertPath: "b.crt", KeyPath: "a.key"},
			},
		}
		require.Error(t, cfg.Validate())
	})
}

func TestCertificateKeyType(t *testing.T) {
	t.Run("default is EC256", func(t *testing.T) {
		cfg := &autocert.Config{Provider: autocert.ProviderLocal}
		require.NoError(t, cfg.Validate())
		_, _, err := cfg.GetLegoConfig()
		require.NoError(t, err)
		require.Equal(t, certcrypto.EC256, cfg.CertKeyType())
	})

	t.Run("rsa2048 alias", func(t *testing.T) {
		cfg := &autocert.Config{Provider: autocert.ProviderLocal, CertificateKeyType: "rsa2048"}
		require.NoError(t, cfg.Validate())
		_, _, err := cfg.GetLegoConfig()
		require.NoError(t, err)
		require.Equal(t, certcrypto.RSA2048, cfg.CertKeyType())
	})

	t.Run("legacy lego tokens still accepted", func(t *testing.T) {
		// lego v5 renamed the KeyType enum values (P256 -> EC256, 2048 -> RSA2048).
		// The old spellings stay valid as godoxy config values.
		for token, want := range map[string]certcrypto.KeyType{
			"P256": certcrypto.EC256,
			"P384": certcrypto.EC384,
			"2048": certcrypto.RSA2048,
			"3072": certcrypto.RSA3072,
			"4096": certcrypto.RSA4096,
			"8192": certcrypto.RSA8192,
		} {
			cfg := &autocert.Config{Provider: autocert.ProviderLocal, CertificateKeyType: token}
			require.NoError(t, cfg.Validate(), token)
			require.Equal(t, want, cfg.CertKeyType(), token)
		}
	})

	t.Run("invalid rejected at validate", func(t *testing.T) {
		cfg := &autocert.Config{Provider: autocert.ProviderLocal, CertificateKeyType: "nope"}
		require.Error(t, cfg.Validate())
	})

	t.Run("extra overrides certificate_key_type", func(t *testing.T) {
		main := &autocert.Config{
			Provider:           autocert.ProviderLocal,
			CertificateKeyType: "EC384",
			Extra:              []autocert.ConfigExtra{{CertPath: "x.crt", KeyPath: "x.key", CertificateKeyType: "RSA4096"}},
		}
		require.NoError(t, main.Validate())
		extra := main.Extra[0].AsConfig()
		_, _, err := extra.GetLegoConfig()
		require.NoError(t, err)
		require.Equal(t, certcrypto.RSA4096, extra.CertKeyType())
	})
}
