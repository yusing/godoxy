package provider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/certs"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/watcher"
	"github.com/yusing/goutils/task"
)

// recoveryTestImpl models an unavailable Docker daemon without depending on a
// host Docker socket. Each successful list returns fresh route objects, just
// as a real Docker listing does.
type recoveryTestImpl struct {
	available atomic.Bool
	loads     atomic.Int32
	logger    zerolog.Logger
}

func (p *recoveryTestImpl) String() string              { return "docker@recovery-test" }
func (p *recoveryTestImpl) ShortName() string           { return "recovery-test" }
func (p *recoveryTestImpl) IsExplicitOnly() bool        { return false }
func (p *recoveryTestImpl) Logger() *zerolog.Logger     { return &p.logger }
func (p *recoveryTestImpl) NewWatcher() watcher.Watcher { return noopWatcher{} }
func (p *recoveryTestImpl) loadRoutesImpl(context.Context) (route.Routes, error) {
	p.loads.Add(1)
	if !p.available.Load() {
		return nil, errors.New("daemon list unavailable")
	}
	return route.Routes{"recovered": {
		Scheme:      route.SchemeHTTP,
		Host:        "example.test",
		Port:        route.Port{Proxy: 8080},
		HealthCheck: health.HealthCheckConfig{Disable: true},
	}}, nil
}

type recoveryTestWatcher struct {
	available *atomic.Bool
	calls     atomic.Int32
}

func (w *recoveryTestWatcher) Watch(task.Parent) watcher.Stream {
	w.calls.Add(1)
	ready := make(chan error, 1)
	if !w.available.Load() {
		ready <- errors.New("daemon ping unavailable")
	}
	close(ready)
	return watcher.Stream{
		Ready:  ready,
		Events: make(chan watcher.Event),
		Errors: make(chan error),
	}
}

func newRecoveryTestProvider(t *testing.T, impl *recoveryTestImpl, w watcher.Watcher) (*Provider, *task.Task) {
	t.Helper()
	runtimeTask := newRecoveryRuntime(t)
	p := &Provider{ProviderImpl: impl, t: routing.ProviderTypeDocker, watcher: w}
	return p, runtimeTask
}

func newRecoveryRuntime(t *testing.T) *task.Task {
	t.Helper()
	oldHTTP, oldHTTPS := common.ProxyHTTPAddr, common.ProxyHTTPSAddr
	common.ProxyHTTPAddr, common.ProxyHTTPSAddr = "127.0.0.1:0", ""
	t.Cleanup(func() { common.ProxyHTTPAddr, common.ProxyHTTPSAddr = oldHTTP, oldHTTPS })
	runtimeTask := task.GetTestTask(t).Subtask("recovery-runtime", false)
	t.Cleanup(func() { runtimeTask.FinishAndWait(nil) })
	entrypoint.SetCtx(runtimeTask, entrypoint.NewEntrypoint(runtimeTask, nil))
	return runtimeTask
}

func waitForRecoveredRoute(t *testing.T, p *Provider) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, ok := p.GetRoute("recovered")
		return ok
	}, 5*time.Second, 10*time.Millisecond)
}

func TestProviderRecoversAfterDockerWatcherAndListBecomeAvailable(t *testing.T) {
	impl := &recoveryTestImpl{logger: zerolog.Nop()}
	w := &recoveryTestWatcher{available: &impl.available}
	p, runtimeTask := newRecoveryTestProvider(t, impl, w)
	require.Error(t, p.LoadRoutes(runtimeTask.Context()))

	activation := p.Activate(runtimeTask)
	require.Error(t, activation.InfrastructureError)
	require.True(t, activation.EventLoopReady, "recoverable watcher failures keep the provider active")
	require.Zero(t, p.NumRoutes())

	impl.available.Store(true)
	waitForRecoveredRoute(t, p)
	_, routed := entrypoint.FromCtx(runtimeTask.Context()).HTTPRoutes().Get("recovered")
	require.True(t, routed, "recovered route must be registered for HTTP traffic")
	require.Eventually(t, func() bool { return w.calls.Load() >= 2 }, time.Second, 10*time.Millisecond,
		"watcher must reconnect after its failed ping")
}

func TestProviderRecoversFailedListWithoutDockerEvents(t *testing.T) {
	impl := &recoveryTestImpl{logger: zerolog.Nop()}
	var watcherAvailable atomic.Bool
	watcherAvailable.Store(true)
	w := &recoveryTestWatcher{available: &watcherAvailable}
	p, runtimeTask := newRecoveryTestProvider(t, impl, w)
	require.Error(t, p.LoadRoutes(runtimeTask.Context()))

	// The watcher is healthy, but no event will ever be delivered. Recovery
	// must retry the failed initial list independently of the event stream.
	activation := p.Activate(runtimeTask)
	require.True(t, activation.EventLoopReady)
	require.Eventually(t, func() bool { return impl.loads.Load() >= 2 }, time.Second, 10*time.Millisecond)
	require.Zero(t, p.NumRoutes())
	impl.available.Store(true)
	waitForRecoveredRoute(t, p)
	_, routed := entrypoint.FromCtx(runtimeTask.Context()).HTTPRoutes().Get("recovered")
	require.True(t, routed, "recovered route must be registered for HTTP traffic")
	require.GreaterOrEqual(t, impl.loads.Load(), int32(3))
}

func TestAgentInitializationRecoversAndPublishesInitializedAgent(t *testing.T) {
	t.Chdir(t.TempDir())
	ca, serverPair, clientPair, err := agent.NewAgent()
	require.NoError(t, err)
	serverCert, err := serverPair.ToTLSCert()
	require.NoError(t, err)
	caPool := x509.NewCertPool()
	require.True(t, caPool.AppendCertsFromPEM(ca.Cert))

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, agent.EndpointInfo):
			// Exercise the legacy discovery path, avoiding stream probes.
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, agent.EndpointName):
			_, _ = w.Write([]byte("recovered-agent"))
		case strings.HasSuffix(r.URL.Path, agent.EndpointVersion):
			_, _ = w.Write([]byte("0.0.0"))
		case strings.HasSuffix(r.URL.Path, agent.EndpointRuntime):
			_, _ = w.Write([]byte("docker"))
		default:
			http.NotFound(w, r)
		}
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{*serverCert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	addr := server.Listener.Addr().String()
	certFile, ok := certs.AgentCertsFilepath(addr)
	require.True(t, ok)
	// Keep the relative certificate archive entirely inside the test directory.
	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), 0o700))
	_, statErr := os.Stat(certFile)
	require.ErrorIs(t, statErr, os.ErrNotExist)

	configured := &agent.AgentConfig{Addr: addr}
	p := NewAgentProvider(configured)
	impl := &recoveryTestImpl{logger: zerolog.Nop()}
	impl.available.Store(true)
	p.ProviderImpl.(*AgentProvider).docker = impl
	w := &recoveryTestWatcher{available: &impl.available}
	p.watcher = w
	runtimeTask := newRecoveryRuntime(t)
	pool := agentpool.NewPool()
	agentpool.SetCtx(runtimeTask, pool)

	require.Error(t, p.LoadRoutes(runtimeTask.Context()), "initialization requires the missing certificate archive")
	require.False(t, pool.Has(configured))
	activation := p.Activate(runtimeTask)
	require.True(t, activation.EventLoopReady)

	archive, err := certs.ZipCert(ca.Cert, clientPair.Cert, clientPair.Key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(certFile, archive, 0o600))
	waitForRecoveredRoute(t, p)
	_, routed := entrypoint.FromCtx(runtimeTask.Context()).HTTPRoutes().Get("recovered")
	require.True(t, routed, "recovered route must be registered for HTTP traffic")
	initialized, ok := pool.GetAgent("recovered-agent")
	require.True(t, ok)
	require.Equal(t, addr, initialized.Addr)
	require.Empty(t, configured.Name, "registry config should remain unmodified")
}

func TestProviderRecoveryStopsAfterCancellation(t *testing.T) {
	impl := &recoveryTestImpl{logger: zerolog.Nop()}
	w := &recoveryTestWatcher{available: &impl.available}
	p, runtimeTask := newRecoveryTestProvider(t, impl, w)
	require.Error(t, p.LoadRoutes(runtimeTask.Context()))
	activation := p.Activate(runtimeTask)
	require.True(t, activation.EventLoopReady)
	runtimeTask.FinishAndWait(context.Canceled)

	loads, watches := impl.loads.Load(), w.calls.Load()
	impl.available.Store(true)
	// A canceled retry is interruptible, so no full backoff should be needed.
	require.False(t, waitProviderRetry(runtimeTask.Context(), 3*time.Second))
	require.Equal(t, loads, impl.loads.Load())
	require.Equal(t, watches, w.calls.Load())
}
