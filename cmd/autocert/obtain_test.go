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
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yusing/godoxy/internal/common"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestRunObtainReportsFlagParseErrors(t *testing.T) {
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
}

func TestGetLegoConfigCustomCACertsSetsTLSMinVersion(t *testing.T) {
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
}

func TestSaveCertCreatesKeyDirectory(t *testing.T) {
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
}

func TestEnsureHTTPTransportForTLSRejectsNonTransportDefault(t *testing.T) {
	oldDefaultTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = oldDefaultTransport
	})

	_, err := ensureHTTPTransportForTLS(&http.Client{})
	require.ErrorContains(t, err, "default transport is")
}

func TestLoadOrCreateACMEKeyReturnsNonNotExistErrors(t *testing.T) {
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
}

func TestSaveCertDoesNotOverwriteExistingPairOnInvalidReplacement(t *testing.T) {
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
