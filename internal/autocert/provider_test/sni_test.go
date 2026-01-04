package provider_test

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
	"github.com/yusing/godoxy/internal/autocert"
)

func writeSelfSignedCert(t *testing.T, dir string, dnsNames []string) (string, string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)

	cn := ""
	if len(dnsNames) > 0 {
		cn = dnsNames[0]
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	require.NoError(t, os.WriteFile(certPath, certPEM, 0o644))
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	return certPath, keyPath
}

func TestGetCertBySNI(t *testing.T) {
	t.Run("extra cert used when main does not match", func(t *testing.T) {
		mainDir := t.TempDir()
		mainCert, mainKey := writeSelfSignedCert(t, mainDir, []string{"*.example.com"})

		extraDir := t.TempDir()
		extraCert, extraKey := writeSelfSignedCert(t, extraDir, []string{"*.internal.example.com"})

		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			CertPath: mainCert,
			KeyPath:  mainKey,
			Extra: []autocert.Config{
				{CertPath: extraCert, KeyPath: extraKey},
			},
		}

		require.NoError(t, cfg.Validate())

		p := autocert.NewProvider(cfg, nil, nil)
		require.NoError(t, p.Setup())

		cert, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "a.internal.example.com"})
		require.NoError(t, err)

		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf.DNSNames, "*.internal.example.com")
	})

	t.Run("exact match wins over wildcard match", func(t *testing.T) {
		mainDir := t.TempDir()
		mainCert, mainKey := writeSelfSignedCert(t, mainDir, []string{"*.example.com"})

		extraDir := t.TempDir()
		extraCert, extraKey := writeSelfSignedCert(t, extraDir, []string{"foo.example.com"})

		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			CertPath: mainCert,
			KeyPath:  mainKey,
			Extra: []autocert.Config{
				{CertPath: extraCert, KeyPath: extraKey},
			},
		}

		require.NoError(t, cfg.Validate())

		p := autocert.NewProvider(cfg, nil, nil)
		require.NoError(t, p.Setup())

		cert, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "foo.example.com"})
		require.NoError(t, err)

		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf.DNSNames, "foo.example.com")
	})

	t.Run("main cert fallback when no match", func(t *testing.T) {
		mainDir := t.TempDir()
		mainCert, mainKey := writeSelfSignedCert(t, mainDir, []string{"*.example.com"})

		extraDir := t.TempDir()
		extraCert, extraKey := writeSelfSignedCert(t, extraDir, []string{"*.test.com"})

		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			CertPath: mainCert,
			KeyPath:  mainKey,
			Extra: []autocert.Config{
				{CertPath: extraCert, KeyPath: extraKey},
			},
		}

		require.NoError(t, cfg.Validate())

		p := autocert.NewProvider(cfg, nil, nil)
		require.NoError(t, p.Setup())

		cert, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "unknown.domain.com"})
		require.NoError(t, err)

		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf.DNSNames, "*.example.com")
	})

	t.Run("nil ServerName returns main cert", func(t *testing.T) {
		mainDir := t.TempDir()
		mainCert, mainKey := writeSelfSignedCert(t, mainDir, []string{"*.example.com"})

		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			CertPath: mainCert,
			KeyPath:  mainKey,
		}

		require.NoError(t, cfg.Validate())

		p := autocert.NewProvider(cfg, nil, nil)
		require.NoError(t, p.Setup())

		cert, err := p.GetCert(nil)
		require.NoError(t, err)

		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf.DNSNames, "*.example.com")
	})

	t.Run("empty ServerName returns main cert", func(t *testing.T) {
		mainDir := t.TempDir()
		mainCert, mainKey := writeSelfSignedCert(t, mainDir, []string{"*.example.com"})

		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			CertPath: mainCert,
			KeyPath:  mainKey,
		}

		require.NoError(t, cfg.Validate())

		p := autocert.NewProvider(cfg, nil, nil)
		require.NoError(t, p.Setup())

		cert, err := p.GetCert(&tls.ClientHelloInfo{ServerName: ""})
		require.NoError(t, err)

		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf.DNSNames, "*.example.com")
	})

	t.Run("case insensitive matching", func(t *testing.T) {
		mainDir := t.TempDir()
		mainCert, mainKey := writeSelfSignedCert(t, mainDir, []string{"*.example.com"})

		extraDir := t.TempDir()
		extraCert, extraKey := writeSelfSignedCert(t, extraDir, []string{"Foo.Example.COM"})

		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			CertPath: mainCert,
			KeyPath:  mainKey,
			Extra: []autocert.Config{
				{CertPath: extraCert, KeyPath: extraKey},
			},
		}

		require.NoError(t, cfg.Validate())

		p := autocert.NewProvider(cfg, nil, nil)
		require.NoError(t, p.Setup())

		cert, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "FOO.EXAMPLE.COM"})
		require.NoError(t, err)

		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf.DNSNames, "Foo.Example.COM")
	})

	t.Run("normalization with trailing dot and whitespace", func(t *testing.T) {
		mainDir := t.TempDir()
		mainCert, mainKey := writeSelfSignedCert(t, mainDir, []string{"*.example.com"})

		extraDir := t.TempDir()
		extraCert, extraKey := writeSelfSignedCert(t, extraDir, []string{"foo.example.com"})

		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			CertPath: mainCert,
			KeyPath:  mainKey,
			Extra: []autocert.Config{
				{CertPath: extraCert, KeyPath: extraKey},
			},
		}

		require.NoError(t, cfg.Validate())

		p := autocert.NewProvider(cfg, nil, nil)
		require.NoError(t, p.Setup())

		cert, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "  foo.example.com.  "})
		require.NoError(t, err)

		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf.DNSNames, "foo.example.com")
	})

	t.Run("longest wildcard match wins", func(t *testing.T) {
		mainDir := t.TempDir()
		mainCert, mainKey := writeSelfSignedCert(t, mainDir, []string{"*.example.com"})

		extraDir1 := t.TempDir()
		extraCert1, extraKey1 := writeSelfSignedCert(t, extraDir1, []string{"*.a.example.com"})

		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			CertPath: mainCert,
			KeyPath:  mainKey,
			Extra: []autocert.Config{
				{CertPath: extraCert1, KeyPath: extraKey1},
			},
		}

		require.NoError(t, cfg.Validate())

		p := autocert.NewProvider(cfg, nil, nil)
		require.NoError(t, p.Setup())

		cert, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "foo.a.example.com"})
		require.NoError(t, err)

		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf.DNSNames, "*.a.example.com")
	})

	t.Run("main cert wildcard match", func(t *testing.T) {
		mainDir := t.TempDir()
		mainCert, mainKey := writeSelfSignedCert(t, mainDir, []string{"*.example.com"})

		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			CertPath: mainCert,
			KeyPath:  mainKey,
		}

		require.NoError(t, cfg.Validate())

		p := autocert.NewProvider(cfg, nil, nil)
		require.NoError(t, p.Setup())

		cert, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "bar.example.com"})
		require.NoError(t, err)

		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf.DNSNames, "*.example.com")
	})

	t.Run("multiple extra certs", func(t *testing.T) {
		mainDir := t.TempDir()
		mainCert, mainKey := writeSelfSignedCert(t, mainDir, []string{"*.example.com"})

		extraDir1 := t.TempDir()
		extraCert1, extraKey1 := writeSelfSignedCert(t, extraDir1, []string{"*.test.com"})

		extraDir2 := t.TempDir()
		extraCert2, extraKey2 := writeSelfSignedCert(t, extraDir2, []string{"*.dev.com"})

		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			CertPath: mainCert,
			KeyPath:  mainKey,
			Extra: []autocert.Config{
				{CertPath: extraCert1, KeyPath: extraKey1},
				{CertPath: extraCert2, KeyPath: extraKey2},
			},
		}

		require.NoError(t, cfg.Validate())

		p := autocert.NewProvider(cfg, nil, nil)
		require.NoError(t, p.Setup())

		cert1, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "foo.test.com"})
		require.NoError(t, err)
		leaf1, err := x509.ParseCertificate(cert1.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf1.DNSNames, "*.test.com")

		cert2, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "bar.dev.com"})
		require.NoError(t, err)
		leaf2, err := x509.ParseCertificate(cert2.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf2.DNSNames, "*.dev.com")
	})

	t.Run("multiple DNSNames in cert", func(t *testing.T) {
		mainDir := t.TempDir()
		mainCert, mainKey := writeSelfSignedCert(t, mainDir, []string{"*.example.com"})

		extraDir := t.TempDir()
		extraCert, extraKey := writeSelfSignedCert(t, extraDir, []string{"foo.example.com", "bar.example.com", "*.test.com"})

		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			CertPath: mainCert,
			KeyPath:  mainKey,
			Extra: []autocert.Config{
				{CertPath: extraCert, KeyPath: extraKey},
			},
		}

		require.NoError(t, cfg.Validate())

		p := autocert.NewProvider(cfg, nil, nil)
		require.NoError(t, p.Setup())

		cert1, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "foo.example.com"})
		require.NoError(t, err)
		leaf1, err := x509.ParseCertificate(cert1.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf1.DNSNames, "foo.example.com")

		cert2, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "bar.example.com"})
		require.NoError(t, err)
		leaf2, err := x509.ParseCertificate(cert2.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf2.DNSNames, "bar.example.com")

		cert3, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "baz.test.com"})
		require.NoError(t, err)
		leaf3, err := x509.ParseCertificate(cert3.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, leaf3.DNSNames, "*.test.com")
	})
}
