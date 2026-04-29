package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-acme/lego/v4/acme"
	"github.com/yusing/godoxy/internal/common"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/registration"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestObtain(t *testing.T) {
	t.Run("runObtain flag parsing", func(t *testing.T) {
		oldStderr := os.Stderr
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stderr = w
		t.Cleanup(func() {
			os.Stderr = oldStderr
			r.Close()
			w.Close()
		})

		err = runObtain([]string{"--bad-flag"})
		require.Error(t, err)
		require.NoError(t, w.Close())

		output, readErr := io.ReadAll(r)
		require.NoError(t, readErr)
		require.Contains(t, string(output), "flag provided but not defined")
	})

	t.Run("obtainCert rejects local providers", func(t *testing.T) {
		for _, provider := range []string{autocert.ProviderLocal, autocert.ProviderPseudo} {
			t.Run(provider, func(t *testing.T) {
				tmpDir := t.TempDir()
				cfg := &autocert.Config{
					Provider: provider,
					CertPath: filepath.Join(tmpDir, "cert.pem"),
					KeyPath:  filepath.Join(tmpDir, "key.pem"),
				}

				err := obtainCert(cfg)
				require.Error(t, err)
				require.ErrorContains(t, err, provider)
				require.ErrorContains(t, err, "cannot obtain ACME certificate")
			})
		}
	})

	t.Run("getLegoConfig custom CA", func(t *testing.T) {
		acmeServer := newTestACMEServer(t)
		defer acmeServer.Close()

		caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: acmeServer.caCert.Raw})
		caCertPath := filepath.Join(t.TempDir(), "ca.pem")
		require.NoError(t, os.WriteFile(caCertPath, caCertPEM, 0o644))
		cfg := &autocert.Config{
			Provider: autocert.ProviderLocal,
			CACerts:  []string{caCertPath},
			HTTPClient: &http.Client{Transport: &http.Transport{
				TLSClientConfig: &tls.Config{},
			}},
		}

		_, legoCfg, err := getLegoConfig(cfg)
		require.NoError(t, err)
		require.NotNil(t, legoCfg.HTTPClient)

		rt, ok := legoCfg.HTTPClient.Transport.(*http.Transport)
		require.True(t, ok)
		require.NotNil(t, rt.TLSClientConfig)
		require.Equal(t, uint16(tls.VersionTLS12), rt.TLSClientConfig.MinVersion)
		require.NotNil(t, rt.TLSClientConfig.RootCAs)
	})

	t.Run("saveCert creates dirs", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &autocert.Config{
			CertPath: filepath.Join(tmpDir, "certs", "cert.pem"),
			KeyPath:  filepath.Join(tmpDir, "keys", "key.pem"),
		}

		certPEM, keyPEM := mustCreateCertificatePair(t)
		cert := &certificate.Resource{
			Certificate: certPEM,
			PrivateKey:  keyPEM,
		}

		require.NoError(t, saveCert(cfg, cert))
		require.FileExists(t, cfg.CertPath)
		require.FileExists(t, cfg.KeyPath)
	})

	t.Run("saveCert overwrite guard", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &autocert.Config{
			CertPath: filepath.Join(tmpDir, "cert.pem"),
			KeyPath:  filepath.Join(tmpDir, "key.pem"),
		}

		originalCert, originalKey := mustCreateCertificatePair(t)
		require.NoError(t, saveCert(cfg, &certificate.Resource{
			Certificate: originalCert,
			PrivateKey:  originalKey,
		}))

		originalCertOnDisk, err := os.ReadFile(cfg.CertPath)
		require.NoError(t, err)
		originalKeyOnDisk, err := os.ReadFile(cfg.KeyPath)
		require.NoError(t, err)

		replacementCert, _ := mustCreateCertificatePair(t)
		_, replacementKey := mustCreateCertificatePair(t)
		err = saveCert(cfg, &certificate.Resource{
			Certificate: replacementCert,
			PrivateKey:  replacementKey,
		})
		require.Error(t, err)

		currentCert, err := os.ReadFile(cfg.CertPath)
		require.NoError(t, err)
		currentKey, err := os.ReadFile(cfg.KeyPath)
		require.NoError(t, err)
		require.True(t, bytes.Equal(originalCertOnDisk, currentCert))
		require.True(t, bytes.Equal(originalKeyOnDisk, currentKey))
	})

	t.Run("saveCert rollback on cert rename fail", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &autocert.Config{
			CertPath: filepath.Join(tmpDir, "cert.pem"),
			KeyPath:  filepath.Join(tmpDir, "key.pem"),
		}

		originalCert, originalKey := mustCreateCertificatePair(t)
		require.NoError(t, saveCert(cfg, &certificate.Resource{
			Certificate: originalCert,
			PrivateKey:  originalKey,
		}))

		originalCertOnDisk, err := os.ReadFile(cfg.CertPath)
		require.NoError(t, err)
		originalKeyOnDisk, err := os.ReadFile(cfg.KeyPath)
		require.NoError(t, err)

		replacementCert, replacementKey := mustCreateCertificatePair(t)
		realRename := osRename
		renameCalls := 0
		osRename = func(oldPath, newPath string) error {
			renameCalls++
			if renameCalls == 4 {
				return fmt.Errorf("forced cert rename failure")
			}
			return realRename(oldPath, newPath)
		}
		t.Cleanup(func() {
			osRename = realRename
		})

		err = saveCert(cfg, &certificate.Resource{
			Certificate: replacementCert,
			PrivateKey:  replacementKey,
		})
		require.Error(t, err)

		currentCert, err := os.ReadFile(cfg.CertPath)
		require.NoError(t, err)
		currentKey, err := os.ReadFile(cfg.KeyPath)
		require.NoError(t, err)
		require.True(t, bytes.Equal(originalKeyOnDisk, currentKey))
		require.True(t, bytes.Equal(originalCertOnDisk, currentCert))
	})

	t.Run("saveCert reports rollback failure on key rename fail", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &autocert.Config{
			CertPath: filepath.Join(tmpDir, "cert.pem"),
			KeyPath:  filepath.Join(tmpDir, "key.pem"),
		}
		originalCert, originalKey := mustCreateCertificatePair(t)
		require.NoError(t, saveCert(cfg, &certificate.Resource{
			Certificate: originalCert,
			PrivateKey:  originalKey,
		}))

		replacementCert, replacementKey := mustCreateCertificatePair(t)
		realRename := osRename
		renameCalls := 0
		osRename = func(oldPath, newPath string) error {
			renameCalls++
			switch renameCalls {
			case 3:
				return fmt.Errorf("forced key rename failure")
			case 5:
				return fmt.Errorf("forced key rollback failure")
			default:
				return realRename(oldPath, newPath)
			}
		}
		t.Cleanup(func() {
			osRename = realRename
		})

		err := saveCert(cfg, &certificate.Resource{
			Certificate: replacementCert,
			PrivateKey:  replacementKey,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "forced key rename failure")
		require.ErrorContains(t, err, "forced key rollback failure")
	})

	t.Run("saveCert reports rollback failure on cert rename fail", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &autocert.Config{
			CertPath: filepath.Join(tmpDir, "cert.pem"),
			KeyPath:  filepath.Join(tmpDir, "key.pem"),
		}
		originalCert, originalKey := mustCreateCertificatePair(t)
		require.NoError(t, saveCert(cfg, &certificate.Resource{
			Certificate: originalCert,
			PrivateKey:  originalKey,
		}))

		replacementCert, replacementKey := mustCreateCertificatePair(t)
		realRename := osRename
		renameCalls := 0
		osRename = func(oldPath, newPath string) error {
			renameCalls++
			switch renameCalls {
			case 4:
				return fmt.Errorf("forced cert rename failure")
			case 6:
				return fmt.Errorf("forced key rollback failure")
			default:
				return realRename(oldPath, newPath)
			}
		}
		t.Cleanup(func() {
			osRename = realRename
		})

		err := saveCert(cfg, &certificate.Resource{
			Certificate: replacementCert,
			PrivateKey:  replacementKey,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "forced cert rename failure")
		require.ErrorContains(t, err, "forced key rollback failure")
	})

	t.Run("ensureHTTPTransportForTLS rejects non-default", func(t *testing.T) {
		oldDefaultTransport := http.DefaultTransport
		http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, nil
		})
		t.Cleanup(func() {
			http.DefaultTransport = oldDefaultTransport
		})

		_, err := ensureHTTPTransportForTLS(&http.Client{})
		require.ErrorContains(t, err, "default transport is")
	})

	t.Run("loadOrCreateACMEKey errors", func(t *testing.T) {
		oldIsTest := common.IsTest
		common.IsTest = false
		t.Cleanup(func() {
			common.IsTest = oldIsTest
		})

		tmpDir := t.TempDir()
		cfg := &autocert.Config{
			Provider:    "cloudflare",
			ACMEKeyPath: tmpDir,
		}

		_, err := loadOrCreateACMEKey(cfg)
		require.Error(t, err)
		require.ErrorContains(t, err, "load ACME key")
	})

	t.Run("safeACMERegistrationFields omits contacts", func(t *testing.T) {
		uri, status := safeACMERegistrationFields(&registration.Resource{
			URI: "https://acme.invalid/acct/123",
			Body: acme.Account{
				Status:  "valid",
				Contact: []string{"mailto:secret@example.com"},
			},
		})

		require.Equal(t, "https://acme.invalid/acct/123", uri)
		require.Equal(t, "valid", status)
	})
}

func mustCreateCertificatePair(t *testing.T) ([]byte, []byte) {
	t.Helper()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := certificateTemplate([]string{"example.com"})
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

func certificateTemplate(dnsNames []string) *x509.Certificate {
	return &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		DNSNames:     dnsNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
}
