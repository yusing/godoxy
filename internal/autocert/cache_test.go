package autocert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeSelfSignedCertToPaths(t *testing.T, certPath, keyPath string, dnsNames []string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)

	cn := ""
	if len(dnsNames) > 0 {
		cn = dnsNames[0]
	}

	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644))
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600))
}

func TestFileCacheGetCertBySNI(t *testing.T) {
	mainDir := t.TempDir()
	mainCert := filepath.Join(mainDir, "main.crt")
	mainKey := filepath.Join(mainDir, "main.key")
	writeSelfSignedCertToPaths(t, mainCert, mainKey, []string{"*.example.com"})

	extraDir := t.TempDir()
	extraCert := filepath.Join(extraDir, "extra.crt")
	extraKey := filepath.Join(extraDir, "extra.key")
	writeSelfSignedCertToPaths(t, extraCert, extraKey, []string{"foo.example.com"})

	cfg := &Config{
		Provider: ProviderLocal,
		CertPath: mainCert,
		KeyPath:  mainKey,
		Extra:    []ConfigExtra{{CertPath: extraCert, KeyPath: extraKey}},
	}

	cache, err := NewFileCache(cfg)
	require.NoError(t, err)
	require.NoError(t, cache.LoadAll())

	cert, err := cache.GetCert(&tls.ClientHelloInfo{ServerName: "foo.example.com"})
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)
	require.Contains(t, leaf.DNSNames, "foo.example.com")
}

func TestFileCacheGetCertInfos(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.crt")
	keyPath := filepath.Join(dir, "cert.key")
	writeSelfSignedCertToPaths(t, certPath, keyPath, []string{"api.example.com"})

	cache, err := NewFileCache(&Config{Provider: ProviderLocal, CertPath: certPath, KeyPath: keyPath})
	require.NoError(t, err)
	require.NoError(t, cache.LoadAll())

	infos, err := cache.GetCertInfos()
	require.NoError(t, err)
	require.Len(t, infos, 1)
	require.Equal(t, "api.example.com", infos[0].Subject)
}

func TestFileCacheReturnsNoCertificatesWhenFilesMissing(t *testing.T) {
	cache, err := NewFileCache(&Config{Provider: ProviderLocal, CertPath: "missing.crt", KeyPath: "missing.key"})
	require.NoError(t, err)

	err = cache.LoadAll()
	require.ErrorIs(t, err, ErrNoCertificates)
}
