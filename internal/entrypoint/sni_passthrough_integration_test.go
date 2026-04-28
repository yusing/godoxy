package entrypoint_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/godoxy/internal/entrypoint"
	epctx "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/route"
	routetypes "github.com/yusing/godoxy/internal/route/types"
	typespkg "github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/task"
)

type staticCertProvider struct{ cert *tls.Certificate }

func (p staticCertProvider) GetCert(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return p.cert, nil
}
func (p staticCertProvider) GetCertInfos() ([]autocert.CertInfo, error) { return nil, nil }
func (p staticCertProvider) ScheduleRenewalAll(task.Parent)             {}
func (p staticCertProvider) ObtainCertAll() error                       { return nil }
func (p staticCertProvider) ForceExpiryAll() bool                       { return false }
func (p staticCertProvider) WaitRenewalDone(context.Context) bool       { return true }

func TestSNIPassthroughSharesHTTPSListener(t *testing.T) {
	cert := newLocalhostCert(t)
	parent := task.GetTestTask(t)
	autocert.SetCtx(parent, staticCertProvider{cert: cert})
	ep := entrypoint.NewEntrypoint(parent, nil)
	epctx.SetCtx(parent, ep)

	listenPort := freeTCPPort(t)

	defaultBindUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("default bind route"))
	}))
	defer defaultBindUpstream.Close()
	defaultBindHost, defaultBindPort := splitHostPort(t, defaultBindUpstream.Listener.Addr().String())

	_, err := route.NewStartedTestRoute(t, &route.Route{
		Alias:  "app-default",
		Scheme: routetypes.SchemeHTTP,
		Host:   defaultBindHost,
		Port: route.Port{
			Listening: listenPort,
			Proxy:     defaultBindPort,
		},
		HealthCheck: typespkg.HealthCheckConfig{Disable: true},
	})
	require.NoError(t, err)

	httpsServer, ok := ep.GetServer(":" + strconv.Itoa(listenPort))
	require.True(t, ok, "default HTTPS server not found")
	require.NotNil(t, httpsServer.FindRoute("app-default.localhost"))

	passthroughUpstream := startTLSEchoServer(t, "passthrough")
	upstreamHost, upstreamPort := splitHostPort(t, passthroughUpstream.Addr().String())

	_, err = route.NewStartedTestRoute(t, &route.Route{
		Alias:       "site2",
		Scheme:      routetypes.SchemeTCP,
		Host:        upstreamHost,
		Port:        route.Port{Listening: listenPort, Proxy: upstreamPort},
		SNIHosts:    []string{"*.site2.domain.tld"},
		HealthCheck: typespkg.HealthCheckConfig{Disable: true},
	})
	require.NoError(t, err)

	passthroughBody := tlsRoundTrip(t, listenPort, "www.site2.domain.tld", "passthrough")
	require.Equal(t, "passthrough", passthroughBody)

	transport := &http.Transport{TLSClientConfig: &tls.Config{
		ServerName:         "app-default.localhost",
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}

	require.Equal(t, "default bind route", httpsGet(t, client, listenPort, "app-default.localhost"))
}

func httpsGet(t *testing.T, client *http.Client, port int, host string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(port)+"/", nil)
	require.NoError(t, err)
	req.Host = host
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	return host, port
}

func newLocalhostCert(t *testing.T) *tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost", "app.localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	return &cert
}

type tlsEchoServer struct {
	ln net.Listener
}

func startTLSEchoServer(t *testing.T, response string) *tlsEchoServer {
	t.Helper()
	cert := newLocalhostCert(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   tls.VersionTLS12,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				buf := make([]byte, len(response))
				_, _ = conn.Read(buf)
				_, _ = conn.Write([]byte(response))
			}()
		}
	}()
	return &tlsEchoServer{ln: ln}
}

func (s *tlsEchoServer) Addr() net.Addr { return s.ln.Addr() }

func tlsRoundTrip(t *testing.T, port int, serverName, payload string) string {
	t.Helper()
	conn, err := tls.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port), &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write([]byte(payload))
	require.NoError(t, err)
	buf := make([]byte, len(payload))
	_, err = conn.Read(buf)
	require.NoError(t, err)
	return string(buf)
}
