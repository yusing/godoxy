package agent

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestNewAgent(t *testing.T) {
	ca, srv, client, err := NewAgent()
	expect.NoError(t, err)
	expect.True(t, ca != nil)
	expect.True(t, srv != nil)
	expect.True(t, client != nil)
}

func TestPEMPair(t *testing.T) {
	ca, srv, client, err := NewAgent()
	expect.NoError(t, err)

	for i, p := range []*PEMPair{ca, srv, client} {
		t.Run(fmt.Sprintf("load-%d", i), func(t *testing.T) {
			var pp PEMPair
			err := pp.Load(p.String())
			expect.NoError(t, err)
			expect.Equal(t, p.Cert, pp.Cert)
			expect.Equal(t, p.Key, pp.Key)
		})
	}
}

func TestPEMPairToTLSCert(t *testing.T) {
	ca, srv, client, err := NewAgent()
	expect.NoError(t, err)

	for i, p := range []*PEMPair{ca, srv, client} {
		t.Run(fmt.Sprintf("toTLSCert-%d", i), func(t *testing.T) {
			cert, err := p.ToTLSCert()
			expect.NoError(t, err)
			expect.True(t, cert != nil)
		})
	}
}

func TestServerClient(t *testing.T) {
	ca, srv, client, err := NewAgent()
	expect.NoError(t, err)

	srvTLS, err := srv.ToTLSCert()
	expect.NoError(t, err)
	expect.True(t, srvTLS != nil)

	clientTLS, err := client.ToTLSCert()
	expect.NoError(t, err)
	expect.True(t, clientTLS != nil)

	caPool := x509.NewCertPool()
	expect.True(t, caPool.AppendCertsFromPEM(ca.Cert))

	srvTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{*srvTLS},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}

	clientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{*clientTLS},
		RootCAs:      caPool,
		ServerName:   CertsDNSName,
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
	expect.NoError(t, err)
	expect.Equal(t, resp.StatusCode, http.StatusOK)
}
