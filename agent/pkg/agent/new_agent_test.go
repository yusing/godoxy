package agent

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestNewAgent(t *testing.T) {
	ca, srv, client, err := NewAgent()
	ExpectNoError(t, err)
	ExpectTrue(t, ca != nil)
	ExpectTrue(t, srv != nil)
	ExpectTrue(t, client != nil)
}

func TestPEMPair(t *testing.T) {
	ca, srv, client, err := NewAgent()
	ExpectNoError(t, err)

	for i, p := range []*PEMPair{ca, srv, client} {
		t.Run(fmt.Sprintf("load-%d", i), func(t *testing.T) {
			var pp PEMPair
			err := pp.Load(p.String())
			ExpectNoError(t, err)
			ExpectBytesEqual(t, p.Cert, pp.Cert)
			ExpectBytesEqual(t, p.Key, pp.Key)
		})
	}
}

func TestPEMPairToTLSCert(t *testing.T) {
	ca, srv, client, err := NewAgent()
	ExpectNoError(t, err)

	for i, p := range []*PEMPair{ca, srv, client} {
		t.Run(fmt.Sprintf("toTLSCert-%d", i), func(t *testing.T) {
			cert, err := p.ToTLSCert()
			ExpectNoError(t, err)
			ExpectTrue(t, cert != nil)
		})
	}
}

func TestServerClient(t *testing.T) {
	ca, srv, client, err := NewAgent()
	ExpectNoError(t, err)

	srvTLS, err := srv.ToTLSCert()
	ExpectNoError(t, err)
	ExpectTrue(t, srvTLS != nil)

	clientTLS, err := client.ToTLSCert()
	ExpectNoError(t, err)
	ExpectTrue(t, clientTLS != nil)

	caPool := x509.NewCertPool()
	ExpectTrue(t, caPool.AppendCertsFromPEM(ca.Cert))

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
	ExpectNoError(t, err)
	ExpectEqual(t, resp.StatusCode, http.StatusOK)
}
