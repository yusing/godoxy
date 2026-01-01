package provider_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
)

func TestExtraCertKeyPathsUnique(t *testing.T) {
	t.Run("duplicate cert_path rejected", func(t *testing.T) {
		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			Extra: []autocert.Config{
				{CertPath: "a.crt", KeyPath: "a.key"},
				{CertPath: "a.crt", KeyPath: "b.key"},
			},
		}
		require.Error(t, cfg.Validate())
	})

	t.Run("duplicate key_path rejected", func(t *testing.T) {
		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			Extra: []autocert.Config{
				{CertPath: "a.crt", KeyPath: "a.key"},
				{CertPath: "b.crt", KeyPath: "a.key"},
			},
		}
		require.Error(t, cfg.Validate())
	})
}
