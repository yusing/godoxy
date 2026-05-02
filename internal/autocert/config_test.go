package autocert_test

import (
	"fmt"
	"testing"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/godoxy/internal/dnsproviders"
	"github.com/yusing/godoxy/internal/serialization"
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
	t.Run("default is EC256 in lego config", func(t *testing.T) {
		cfg := &autocert.Config{Provider: autocert.ProviderLocal}
		require.NoError(t, cfg.Validate())
		_, legoCfg, err := cfg.GetLegoConfig()
		require.NoError(t, err)
		require.Equal(t, certcrypto.EC256, legoCfg.Certificate.KeyType)
	})

	t.Run("rsa2048 alias", func(t *testing.T) {
		cfg := &autocert.Config{Provider: autocert.ProviderLocal, CertificateKeyType: "rsa2048"}
		require.NoError(t, cfg.Validate())
		_, legoCfg, err := cfg.GetLegoConfig()
		require.NoError(t, err)
		require.Equal(t, certcrypto.RSA2048, legoCfg.Certificate.KeyType)
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
		_, legoCfg, err := extra.GetLegoConfig()
		require.NoError(t, err)
		require.Equal(t, certcrypto.RSA4096, legoCfg.Certificate.KeyType)
	})
}
