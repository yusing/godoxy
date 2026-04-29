package main

import (
	"testing"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
)

func TestCertificateKeyType(t *testing.T) {
	t.Run("default is EC256 in lego config", func(t *testing.T) {
		cfg := &autocert.Config{Provider: autocert.ProviderLocal}
		require.NoError(t, cfg.Validate())
		_, legoCfg, err := getLegoConfig(cfg)
		require.NoError(t, err)
		require.Equal(t, certcrypto.EC256, legoCfg.Certificate.KeyType)
	})

	t.Run("rsa2048 alias", func(t *testing.T) {
		cfg := &autocert.Config{Provider: autocert.ProviderLocal, CertificateKeyType: "rsa2048"}
		require.NoError(t, cfg.Validate())
		_, legoCfg, err := getLegoConfig(cfg)
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
		_, legoCfg, err := getLegoConfig(extra)
		require.NoError(t, err)
		require.Equal(t, certcrypto.RSA4096, legoCfg.Certificate.KeyType)
	})
}
