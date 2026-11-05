package provider

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	agentcert "github.com/yusing/godoxy/agent/pkg/agent"
)

func TestFileReloadReadsTLSFiles(t *testing.T) {
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) != 0 {
			_, _ = io.WriteString(w, r.TLS.PeerCertificates[0].SerialNumber.String())
		}
	}))
	upstream.TLS = &tls.Config{ClientAuth: tls.RequestClientCert}
	upstream.StartTLS()
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	dir := t.TempDir()
	ca, cert, key := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "client.pem"), filepath.Join(dir, "client.key")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: upstream.Certificate().Raw}), 0o600))
	writeClient := func() string {
		t.Helper()
		_, _, client, err := agentcert.NewAgent()
		require.NoError(t, err)
		block, _ := pem.Decode(client.Cert)
		require.NotNil(t, block)
		leaf, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(cert, client.Cert, 0o600))
		require.NoError(t, os.WriteFile(key, client.Key, 0o600))
		return leaf.SerialNumber.String()
	}
	firstClient := writeClient()
	base := fmt.Sprintf("app:\n  scheme: https\n  host: %s\n  port: %s\n  healthcheck:\n    disabled: true\n  ssl_trusted_certificate: %s\n", target.Hostname(), target.Port(), ca)
	for _, clientCert := range []bool{false, true} {
		t.Run(fmt.Sprintf("client-certificate=%t", clientCert), func(t *testing.T) {
			definition := base
			if clientCert {
				definition += fmt.Sprintf("  ssl_certificate: %s\n  ssl_certificate_key: %s\n", cert, key)
			}
			p, root, ep, _ := fileReloadFixture(t, definition)
			response := func() string {
				t.Helper()
				r, ok := ep.HTTPRoutes().Get("app")
				require.True(t, ok)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://app/", nil))
				require.Equal(t, http.StatusOK, w.Code)
				return w.Body.String()
			}
			before, _ := p.lockGetRoute("app")
			require.Empty(t, before.SourceFingerprint())
			var nextClient string
			if clientCert {
				require.Equal(t, firstClient, response())
				nextClient = writeClient()
			} else {
				require.Empty(t, response())
			}
			reloadFile(p, root)
			after, _ := p.lockGetRoute("app")
			require.NotSame(t, before, after, "external TLS dependencies must be reloaded even when the definition is unchanged")
			if clientCert {
				require.Equal(t, nextClient, response())
			} else {
				require.Empty(t, response())
			}
		})
	}
}
