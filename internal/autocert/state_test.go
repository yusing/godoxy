package autocert

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestObtainCertUsesStandaloneBinaryAndReloadsCertificate(t *testing.T) {
	oldUseLocal := useLocalAutocertOperations.Load()
	useLocalAutocertOperations.Store(false)
	oldRunner := autocertCommandRunner
	t.Cleanup(func() {
		useLocalAutocertOperations.Store(oldUseLocal)
		autocertCommandRunner = oldRunner
	})

	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	var called bool
	autocertCommandRunner = func(ctx context.Context, name string, args ...string) error {
		called = true
		require.Equal(t, context.Background(), ctx)
		require.Equal(t, "autocert", name)
		require.True(t, slices.Contains(args, "obtain"))
		require.True(t, slices.Contains(args, certPath))
		return writeTestCertificate(certPath, keyPath, []string{"example.com"})
	}

	provider, err := NewProvider(&Config{
		Provider: "cloudflare",
		Domains:  []string{"example.com"},
		CertPath: certPath,
		KeyPath:  keyPath,
	})
	require.NoError(t, err)

	require.NoError(t, provider.ObtainCert(t.Context()))
	require.True(t, called)
	require.NotNil(t, provider.getTLSCert())
	require.Contains(t, provider.GetExpiries(), "example.com")
}

func writeTestCertificate(certPath, keyPath string, dnsNames []string) error {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		DNSNames:     dnsNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &privKey.PublicKey, privKey)
	if err != nil {
		return err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return err
	}
	return nil
}
