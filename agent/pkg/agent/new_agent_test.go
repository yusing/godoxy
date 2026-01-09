package agent

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent/common"
)

func TestNewAgent(t *testing.T) {
	ca, srv, client, err := NewAgent()
	require.NoError(t, err)
	require.NotNil(t, ca)
	require.NotNil(t, srv)
	require.NotNil(t, client)
}

func TestPEMPair(t *testing.T) {
	ca, srv, client, err := NewAgent()
	require.NoError(t, err)

	for i, p := range []*PEMPair{ca, srv, client} {
		t.Run(fmt.Sprintf("load-%d", i), func(t *testing.T) {
			var pp PEMPair
			err := pp.Load(p.String())
			require.NoError(t, err)
			require.Equal(t, p.Cert, pp.Cert)
			require.Equal(t, p.Key, pp.Key)
		})
	}
}

func TestPEMPairToTLSCert(t *testing.T) {
	ca, srv, client, err := NewAgent()
	require.NoError(t, err)

	for i, p := range []*PEMPair{ca, srv, client} {
		t.Run(fmt.Sprintf("toTLSCert-%d", i), func(t *testing.T) {
			cert, err := p.ToTLSCert()
			require.NoError(t, err)
			require.NotNil(t, cert)
		})
	}
}

func TestServerClient(t *testing.T) {
	ca, srv, client, err := NewAgent()
	require.NoError(t, err)

	srvTLS, err := srv.ToTLSCert()
	require.NoError(t, err)
	require.NotNil(t, srvTLS)

	clientTLS, err := client.ToTLSCert()
	require.NoError(t, err)
	require.NotNil(t, clientTLS)

	caPool := x509.NewCertPool()
	require.True(t, caPool.AppendCertsFromPEM(ca.Cert))

	srvTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{*srvTLS},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}

	clientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{*clientTLS},
		RootCAs:      caPool,
		ServerName:   common.CertsDNSName,
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = srvTLSConfig
	server.StartTLS()
	defer server.Close()

	httpClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: clientTLSConfig},
	}

	resp, err := httpClient.Get(server.URL)
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)
}

func TestPEMPairEncryptDecrypt(t *testing.T) {
	encKey := make([]byte, 32)
	_, err := rand.Read(encKey)
	require.NoError(t, err)

	ca, _, _, err := NewAgent()
	require.NoError(t, err)

	encCA, err := ca.Encrypt(encKey)
	require.NoError(t, err)
	require.NotNil(t, encCA)

	decCA, err := encCA.Decrypt(encKey)
	require.NoError(t, err)
	require.NotNil(t, decCA)

	require.Equal(t, string(ca.Cert), string(decCA.Cert))
	require.Equal(t, string(ca.Key), string(decCA.Key))
}
