package entrypoint

import (
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/testcert"
	"github.com/yusing/goutils/task"
)

type startupCertProvider struct {
	autocert.Provider
	calls, failAt int
	err           error
	beforeFailure func()
}

func (p *startupCertProvider) GetCert(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	p.calls++
	if p.calls >= p.failAt {
		if p.beforeFailure != nil {
			p.beforeFailure()
		}
		return nil, p.err
	}
	return p.Provider.GetCert(hello)
}

func TestHTTPSStartupFailurePreservesSNIPassthrough(t *testing.T) {
	previous := common.SNIRoutingForTCPRoutes
	common.SNIRoutingForTCPRoutes = true
	t.Cleanup(func() { common.SNIRoutingForTCPRoutes = previous })
	for _, tc := range []struct {
		name   string
		failAt int
		err    error
	}{
		{name: "missing provider"},
		{name: "nil certificate", failAt: 1},
		{name: "certificate error", failAt: 1, err: errors.New("certificate unavailable")},
		{name: "startup fails after listener acquisition", failAt: 2, err: errors.New("certificate unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.failAt != 0 {
				autocert.SetCtx(task.GetTestTask(t), &startupCertProvider{
					Provider: testcert.NewProvider(t), failAt: tc.failAt, err: tc.err,
				})
			}
			ep := NewTestEntrypoint(t, nil)
			t.Cleanup(func() { ep.Task().FinishAndWait(nil) })
			addr := reserveSNIListenAddr(t)
			proxied := make(chan struct{}, 1)
			stream := &testConnProxyStream{proxyConn: func(conn net.Conn) {
				_ = conn.Close()
				proxied <- struct{}{}
			}}
			route := newFakeSNIStreamRoute(t, "passthrough.example.com", addr, stream)
			require.NoError(t, ep.sni.AddRoute(route))
			listener, ok := ep.sni.listeners.Load(addr)
			require.True(t, ok)

			require.Error(t, newHTTPServer(ep).Listen(addr, HTTPProtoHTTPS))
			retained, ok := ep.sni.listeners.Load(addr)
			require.True(t, ok)
			require.Same(t, listener, retained)

			// An actual ClientHello must still reach the passthrough route.
			_, _ = tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", addr,
				&tls.Config{ServerName: "passthrough.example.com"})
			select {
			case <-proxied:
			case <-time.After(time.Second):
				t.Fatal("failed HTTPS startup disabled passthrough")
			}
		})
	}
}

func TestHTTPSStartupFailureReleasesNewListener(t *testing.T) {
	previous := common.SNIRoutingForTCPRoutes
	common.SNIRoutingForTCPRoutes = true
	t.Cleanup(func() { common.SNIRoutingForTCPRoutes = previous })
	autocert.SetCtx(task.GetTestTask(t), &startupCertProvider{
		Provider: testcert.NewProvider(t), failAt: 2, err: errors.New("certificate unavailable"),
	})
	ep := NewTestEntrypoint(t, nil)
	t.Cleanup(func() { ep.Task().FinishAndWait(nil) })
	addr := reserveSNIListenAddr(t)
	require.Error(t, newHTTPServer(ep).Listen(addr, HTTPProtoHTTPS))
	require.Zero(t, ep.sni.listeners.Size())
	listener, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	require.NoError(t, listener.Close())
}

func TestConcurrentSNIRouteRegistrationSurvivesHTTPSFailure(t *testing.T) {
	previous := common.SNIRoutingForTCPRoutes
	common.SNIRoutingForTCPRoutes = true
	t.Cleanup(func() { common.SNIRoutingForTCPRoutes = previous })
	entered, resume := make(chan struct{}), make(chan struct{})
	resumeFailure := sync.OnceFunc(func() { close(resume) })
	t.Cleanup(resumeFailure)
	autocert.SetCtx(task.GetTestTask(t), &startupCertProvider{
		Provider: testcert.NewProvider(t), failAt: 2, err: errors.New("certificate unavailable"),
		beforeFailure: func() { close(entered); <-resume },
	})
	ep := NewTestEntrypoint(t, nil)
	t.Cleanup(func() { resumeFailure(); ep.Task().FinishAndWait(nil) })
	addr := reserveSNIListenAddr(t)
	failed := make(chan error, 1)
	go func() { failed <- newHTTPServer(ep).Listen(addr, HTTPProtoHTTPS) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("HTTPS startup did not reach certificate validation")
	}
	proxied := make(chan struct{}, 1)
	stream := &testConnProxyStream{proxyConn: func(conn net.Conn) {
		_ = conn.Close()
		proxied <- struct{}{}
	}}
	route := newFakeSNIStreamRoute(t, "passthrough.example.com", addr, stream)
	registered := make(chan error, 1)
	go func() { registered <- ep.sni.AddRoute(route) }()
	resumeFailure()
	require.Error(t, <-failed)
	require.NoError(t, <-registered)
	_, _ = tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", addr,
		&tls.Config{ServerName: "passthrough.example.com"})
	select {
	case <-proxied:
	case <-time.After(time.Second):
		t.Fatal("concurrently registered passthrough route lost its listener")
	}
}
