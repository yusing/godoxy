package autocert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/goutils/task"
)

type offlineTransport struct{ attempts atomic.Int32 }

func (tr *offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	tr.attempts.Add(1)
	return nil, errors.New("DNS unavailable")
}

func cachedProvider(t *testing.T, expiry time.Time) (*Provider, *offlineTransport) {
	t.Helper()
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "example.com"},
		DNSNames:     []string{"example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     expiry,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	tr := new(offlineTransport)
	cfg := &Config{
		Provider:    ProviderCustom,
		Domains:     []string{"example.com"},
		CertPath:    filepath.Join(dir, "cert.pem"),
		KeyPath:     filepath.Join(dir, "key.pem"),
		ACMEKeyPath: filepath.Join(dir, "acme.key"),
		HTTPClient:  &http.Client{Transport: tr},
	}
	require.NoError(t, os.WriteFile(cfg.CertPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(cfg.KeyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	user, legoCfg, err := cfg.GetLegoConfig()
	require.NoError(t, err)
	p, err := NewProvider(cfg, user, legoCfg)
	require.NoError(t, err)
	return p, tr
}

func TestCachedCertificateDoesNotRequireACME(t *testing.T) {
	p, tr := cachedProvider(t, time.Now().Add(75*24*time.Hour))
	require.NoError(t, p.ObtainCertIfNotExistsAll(t.Context()))
	require.Zero(t, tr.attempts.Load())
	cert, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "example.com"})
	require.NoError(t, err)
	require.NoError(t, cert.Leaf.VerifyHostname("example.com"))

	// Exercise a real TLS handshake using only the cached certificate.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		serverDone <- tls.Server(conn, &tls.Config{GetCertificate: p.GetCert}).HandshakeContext(t.Context())
	}()
	roots := x509.NewCertPool()
	roots.AddCert(cert.Leaf)
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", listener.Addr().String(), &tls.Config{
		RootCAs: roots, ServerName: "example.com",
	})
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	require.NoError(t, <-serverDone)
	require.Zero(t, tr.attempts.Load())

	// Missing certificates still report the actual issuance failure.
	require.NoError(t, os.Remove(p.cfg.CertPath))
	require.ErrorContains(t, p.ObtainCertIfNotExistsAll(t.Context()), "DNS unavailable")
	require.Positive(t, tr.attempts.Load())
}

func TestCachedExtraCertificatesDoNotRequireACME(t *testing.T) {
	main, mainTransport := cachedProvider(t, time.Now().Add(75*24*time.Hour))
	extra, extraTransport := cachedProvider(t, time.Now().Add(75*24*time.Hour))
	main.cfg.Extra = []ConfigExtra{ConfigExtra(*extra.cfg)}
	p, err := NewProvider(main.cfg, main.user, main.legoCfg)
	require.NoError(t, err)
	require.NoError(t, p.ObtainCertIfNotExistsAll(t.Context()))
	infos, err := p.GetCertInfos()
	require.NoError(t, err)
	require.Len(t, infos, 2)
	require.Zero(t, mainTransport.attempts.Load())
	require.Zero(t, extraTransport.attempts.Load())
}

func TestScheduledRenewalRetriesDirectoryFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, tr := cachedProvider(t, time.Now().Add(20*24*time.Hour))
		require.NoError(t, p.ObtainCertIfNotExistsAll(t.Context()))
		parent := task.GetTestTask(t).Subtask("renewal-test", true)
		defer parent.Finish(nil)
		p.ScheduleRenewalAll(parent)
		synctest.Wait()
		require.EqualValues(t, 1, tr.attempts.Load())
		time.Sleep(renewalCooldownDuration)
		synctest.Wait()
		require.EqualValues(t, 2, tr.attempts.Load())
		_, err := p.GetCert(&tls.ClientHelloInfo{ServerName: "example.com"})
		require.NoError(t, err)
	})
}

func TestFailedEarlyForcedRenewalPreservesScheduledRenewal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, tr := cachedProvider(t, time.Now().Add(75*24*time.Hour))
		require.NoError(t, p.ObtainCertIfNotExistsAll(t.Context()))
		parent := task.GetTestTask(t).Subtask("renewal-test", true)
		defer parent.Finish(nil)
		p.ScheduleRenewalAll(parent)
		require.True(t, p.ForceExpiryAll())
		synctest.Wait()
		require.EqualValues(t, 1, tr.attempts.Load())

		time.Sleep(renewalCooldownDuration)
		synctest.Wait()
		require.EqualValues(t, 1, tr.attempts.Load(), "not yet due for automatic renewal")

		time.Sleep(time.Until(p.ShouldRenewOn()))
		synctest.Wait()
		require.EqualValues(t, 2, tr.attempts.Load(), "automatic renewal must remain scheduled")
	})
}

func TestGetCertWithoutCertificateFailsImmediately(t *testing.T) {
	p, err := NewProvider(&Config{Provider: ProviderLocal}, nil, nil)
	require.NoError(t, err)
	_, err = p.GetCert(&tls.ClientHelloInfo{ServerName: "example.com"})
	require.ErrorIs(t, err, ErrNoCertificates)
}
