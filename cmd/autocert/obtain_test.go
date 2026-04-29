package main

import (
	"crypto/tls"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

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

	cert := &certificate.Resource{
		Certificate: []byte("cert"),
		PrivateKey:  []byte("key"),
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
