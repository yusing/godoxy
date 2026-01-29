package autocert_test

import (
	"fmt"
	"testing"

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
