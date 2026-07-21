package routeimpl_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agentproxy"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/homepage"
	"github.com/yusing/godoxy/internal/loadbalancer"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/godoxy/internal/routeimpl"
	"github.com/yusing/godoxy/internal/routetest"
	"github.com/yusing/godoxy/internal/routevalidate"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/serialization"
	"github.com/yusing/goutils/http/reverseproxy"
	"github.com/yusing/goutils/server"
	"github.com/yusing/goutils/task"
	"github.com/yusing/goutils/version"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func TestMain(m *testing.M) {
	route.InitBuilder(routevalidate.Validate)
	os.Exit(m.Run())
}

type testPool[T interface{ Key() string }] struct {
	mu    sync.RWMutex
	items map[string]T
}

func newTestPool[T interface{ Key() string }]() *testPool[T] {
	return &testPool[T]{items: make(map[string]T)}
}

func (p *testPool[T]) Get(alias string) (T, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	v, ok := p.items[alias]
	return v, ok
}

func (p *testPool[T]) Iter(yield func(alias string, r T) bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for alias, r := range p.items {
		if !yield(alias, r) {
			return
		}
	}
}

func (p *testPool[T]) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.items)
}

func (p *testPool[T]) Add(r T) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items[r.Key()] = r
}

func (p *testPool[T]) Del(r T) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.items, r.Key())
}

type testEntrypoint struct {
	httpRoutes     *testPool[routing.HTTPRoute]
	streamRoutes   *testPool[routing.StreamRoute]
	excludedRoutes *testPool[routing.Route]
}

func newTestEntrypoint() *testEntrypoint {
	return &testEntrypoint{
		httpRoutes:     newTestPool[routing.HTTPRoute](),
		streamRoutes:   newTestPool[routing.StreamRoute](),
		excludedRoutes: newTestPool[routing.Route](),
	}
}

func (ep *testEntrypoint) ProxyProtocolPolicy() (server.ProxyProtocolPolicy, error) {
	return server.ProxyProtocolPolicy{}, nil
}
func (ep *testEntrypoint) DisablePoolsLog(bool) {}

func (ep *testEntrypoint) GetRoute(alias string) (routing.Route, bool) {
	if r, ok := ep.httpRoutes.Get(alias); ok {
		return r, true
	}
	if r, ok := ep.streamRoutes.Get(alias); ok {
		return r, true
	}
	if r, ok := ep.excludedRoutes.Get(alias); ok {
		return r, true
	}
	return nil, false
}

func (ep *testEntrypoint) StartAddRoute(r routing.Route) error {
	if r.ShouldExclude() {
		ep.excludedRoutes.Add(r)
		return nil
	}
	switch rt := r.(type) {
	case routing.HTTPRoute:
		ep.httpRoutes.Add(rt)
		return nil
	case routing.StreamRoute:
		ep.streamRoutes.Add(rt)
		return nil
	default:
		return fmt.Errorf("unknown route type: %T", r)
	}
}

func (ep *testEntrypoint) IterRoutes(yield func(r routing.Route) bool) {
	ep.httpRoutes.Iter(func(_ string, r routing.HTTPRoute) bool {
		return yield(r)
	})
	ep.streamRoutes.Iter(func(_ string, r routing.StreamRoute) bool {
		return yield(r)
	})
	ep.excludedRoutes.Iter(func(_ string, r routing.Route) bool {
		return yield(r)
	})
}

func (ep *testEntrypoint) NumRoutes() int {
	return ep.httpRoutes.Size() + ep.streamRoutes.Size() + ep.excludedRoutes.Size()
}

func (ep *testEntrypoint) RoutesByProvider() map[string][]routing.Route {
	return map[string][]routing.Route{}
}

func (ep *testEntrypoint) HTTPRoutes() routing.PoolLike[routing.HTTPRoute] {
	return ep.httpRoutes
}

func (ep *testEntrypoint) StreamRoutes() routing.PoolLike[routing.StreamRoute] {
	return ep.streamRoutes
}

func (ep *testEntrypoint) ExcludedRoutes() routing.RWPoolLike[routing.Route] {
	return ep.excludedRoutes
}

func (ep *testEntrypoint) GetHealthInfo() map[string]health.HealthInfo {
	return nil
}

func (ep *testEntrypoint) GetHealthInfoWithoutDetail() map[string]health.HealthInfoWithoutDetail {
	return nil
}

func (ep *testEntrypoint) GetHealthInfoSimple() map[string]health.HealthStatus {
	return nil
}

func TestReverseProxyRoute(t *testing.T) {
	t.Run("LinkToLoadBalancer", func(t *testing.T) {
		testTask := task.GetTestTask(t)
		entrypoint.SetCtx(testTask, newTestEntrypoint())

		cfg := route.Route{
			Alias:  "test",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80},
			LoadBalance: &loadbalancer.Config{
				Link: "test",
			},
		}
		cfg1 := route.Route{
			Alias:  "test1",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80},
			LoadBalance: &loadbalancer.Config{
				Link: "test",
			},
		}
		r, err := routetest.NewStartedRoute(t, &cfg)
		require.NoError(t, err)
		assert.NotNil(t, r)
		r2, err := routetest.NewStartedRoute(t, &cfg1)
		require.NoError(t, err)
		assert.NotNil(t, r2)
	})
	t.Run("LoadBalancerRoute", func(t *testing.T) {
		testTask := task.GetTestTask(t)
		entrypoint.SetCtx(testTask, newTestEntrypoint())

		newServer := func() *httptest.Server {
			return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
		}

		srv1 := newServer()
		t.Cleanup(srv1.Close)
		srv2 := newServer()
		t.Cleanup(srv2.Close)
		srv3 := newServer()
		t.Cleanup(srv3.Close)

		makeRoute := func(alias string, target *httptest.Server) *route.Route {
			t.Helper()

			targetURL, err := url.Parse(target.URL)
			require.NoError(t, err)

			host, portStr, err := net.SplitHostPort(targetURL.Host)
			require.NoError(t, err)
			port, err := strconv.Atoi(portStr)
			require.NoError(t, err)

			return &route.Route{
				Alias:  alias,
				Scheme: route.SchemeHTTP,
				Host:   host,
				Port:   route.Port{Proxy: port},
				Homepage: &homepage.ItemConfig{
					Show: true,
				},
				LoadBalance: &loadbalancer.Config{
					Link: "lb-test",
				},
				HealthCheck: health.HealthCheckConfig{
					Path:     "/",
					Interval: 2 * time.Second,
					Timeout:  time.Second,
					UseGet:   true,
				},
			}
		}

		_, err := routetest.NewStartedRoute(t, makeRoute("lb-1", srv1))
		require.NoError(t, err)
		_, err = routetest.NewStartedRoute(t, makeRoute("lb-2", srv2))
		require.NoError(t, err)
		_, err = routetest.NewStartedRoute(t, makeRoute("lb-3", srv3))
		require.NoError(t, err)

		ep := entrypoint.FromCtx(testTask.Context())
		require.NotNil(t, ep)

		lbRoute, ok := ep.HTTPRoutes().Get("lb-test")
		require.True(t, ok)

		lb, ok := lbRoute.(*routeimpl.ReverseProxyRoute)
		require.True(t, ok)
		require.False(t, lb.ShouldExclude())
		require.NotNil(t, lb.LoadBalancer())
		require.NotNil(t, lb.HealthMonitor())
		assert.Equal(t, route.SchemeNone, lb.Scheme)
		assert.Empty(t, lb.Host)
		assert.Zero(t, lb.Port.Proxy)
		assert.Equal(t, "3/3 servers are healthy", lb.HealthMonitor().Detail())
	})
}

func TestReverseProxyRoute_RulesRouteCommandPreservesH2CUpstream(t *testing.T) {
	testTask := task.GetTestTask(t)
	entrypoint.SetCtx(testTask, newTestEntrypoint())

	gotBackendProto := make(chan int, 1)
	backend := httptest.NewUnstartedServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBackendProto <- r.ProtoMajor
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0, 0, 0, 0, 0})
		w.Header().Set("Grpc-Status", "0")
	}), &http2.Server{}))
	backend.Start()
	t.Cleanup(backend.Close)

	backendURL, err := url.Parse(backend.URL)
	require.NoError(t, err)
	backendPort, err := strconv.Atoi(backendURL.Port())
	require.NoError(t, err)

	grpcRoute, err := routetest.NewStartedRoute(t, &route.Route{
		Alias:       "netbird-grpc",
		Scheme:      route.SchemeH2C,
		Host:        backendURL.Hostname(),
		Port:        route.Port{Proxy: backendPort},
		HealthCheck: health.HealthCheckConfig{Disable: true},
	})
	require.NoError(t, err)
	require.NotNil(t, grpcRoute)

	var frontendRules rules.Rules
	_, err = serialization.ConvertString(strings.TrimSpace(`
header Content-Type glob(application/grpc*) {
  route netbird-grpc
}
default {
  pass
}
`), reflect.ValueOf(&frontendRules))
	require.NoError(t, err)

	frontend, err := routetest.NewStartedRoute(t, &route.Route{
		Alias:       "netbird",
		Scheme:      route.SchemeHTTP,
		Host:        "example.com",
		Port:        route.Port{Proxy: 80},
		Rules:       frontendRules,
		HealthCheck: health.HealthCheckConfig{Disable: true},
	})
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(testTask.Context(), http.MethodPost, "http://netbird.local/management.ManagementService/GetServerKey", strings.NewReader("grpc-body"))
	req.Header.Set("Content-Type", "application/grpc+proto")
	rec := httptest.NewRecorder()

	frontend.(routing.HTTPRoute).ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	_, _ = io.ReadAll(res.Body)

	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "application/grpc", res.Header.Get("Content-Type"))
	require.Equal(t, "0", res.Trailer.Get("Grpc-Status"))

	select {
	case proto := <-gotBackendProto:
		require.Equal(t, 2, proto)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for backend request")
	}
}

func TestReverseProxyRoute_RulesRouteCommandFlushesHeaderOnlyH2CStream(t *testing.T) {
	testTask := task.GetTestTask(t)
	entrypoint.SetCtx(testTask, newTestEntrypoint())

	backend := httptest.NewUnstartedServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("X-Wiretrustee-Peer-Registered", "1")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}), &http2.Server{}))
	backend.Start()
	t.Cleanup(backend.Close)

	backendURL, err := url.Parse(backend.URL)
	require.NoError(t, err)
	backendPort, err := strconv.Atoi(backendURL.Port())
	require.NoError(t, err)

	grpcRoute, err := routetest.NewStartedRoute(t, &route.Route{
		Alias:       "netbird-grpc",
		Scheme:      route.SchemeH2C,
		Host:        backendURL.Hostname(),
		Port:        route.Port{Proxy: backendPort},
		HealthCheck: health.HealthCheckConfig{Disable: true},
	})
	require.NoError(t, err)
	require.NotNil(t, grpcRoute)

	var frontendRules rules.Rules
	_, err = serialization.ConvertString(strings.TrimSpace(`
path glob(/signalexchange.SignalExchange/*) {
  route netbird-grpc
}
default {
  pass
}
`), reflect.ValueOf(&frontendRules))
	require.NoError(t, err)

	frontend, err := routetest.NewStartedRoute(t, &route.Route{
		Alias:       "netbird",
		Scheme:      route.SchemeHTTP,
		Host:        "example.com",
		Port:        route.Port{Proxy: 80},
		Rules:       frontendRules,
		HealthCheck: health.HealthCheckConfig{Disable: true},
	})
	require.NoError(t, err)

	proxy := httptest.NewUnstartedServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		frontend.(routing.HTTPRoute).ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), routing.EntrypointContextKey{}, entrypoint.FromCtx(testTask.Context()))))
	}), &http2.Server{}))
	proxy.Start()
	t.Cleanup(proxy.Close)

	client := &http.Client{Transport: &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}}

	ctx, cancel := context.WithTimeout(testTask.Context(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, proxy.URL+"/signalexchange.SignalExchange/ConnectStream", nil)
	require.NoError(t, err)
	req.Host = "netbird.local"
	req.Header.Set("Content-Type", "application/grpc")

	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "1", res.Header.Get("X-Wiretrustee-Peer-Registered"))
}

func TestReverseProxyRoute_AgentProxyConfigHeadersPreserveH2CScheme(t *testing.T) {
	targetURL, err := url.Parse("h2c://netbird-server:80")
	require.NoError(t, err)

	for _, tt := range []struct {
		name       string
		agentVer   version.Version
		wantScheme string
		wantHost   string
		wantLegacy bool
		wantModern bool
	}{
		{
			name:       "legacy agents only support http/https header",
			agentVer:   version.New(0, 18, 5),
			wantScheme: "http",
			wantHost:   "netbird-server:80",
			wantLegacy: true,
		},
		{
			name:       "modern agents receive full scheme header",
			agentVer:   version.New(0, 28, 1),
			wantScheme: "h2c",
			wantHost:   "netbird-server:80",
			wantModern: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				cfg, err := agentproxy.ConfigFromHeaders(req.Header)
				require.NoError(t, err)
				require.Equal(t, tt.wantScheme, cfg.Scheme)
				require.Equal(t, tt.wantHost, cfg.Host)
				require.Equal(t, tt.wantLegacy, req.Header.Get(agentproxy.HeaderXProxyHTTPS) != "")
				require.Equal(t, tt.wantModern, req.Header.Get(agentproxy.HeaderXProxyScheme) != "")

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("ok")),
					Request:    req,
				}, nil
			})
			agentURL, err := url.Parse("https://agent.local/godoxy/agent/proxy/http")
			require.NoError(t, err)

			rp := reverseproxy.NewReverseProxy("agent-h2c", agentURL, transport)
			cfg := agentproxy.Config{
				Scheme: "h2c",
				Host:   targetURL.Host,
			}
			setHeaderFunc := cfg.SetAgentProxyConfigHeadersLegacy
			if !tt.agentVer.IsOlderThan(version.New(0, 18, 6)) {
				setHeaderFunc = cfg.SetAgentProxyConfigHeaders
			}

			ori := rp.HandlerFunc
			rp.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
				setHeaderFunc(r.Header)
				ori(w, r)
			}

			req := httptest.NewRequest(http.MethodPost, "http://proxy.local/management.ManagementService/GetServerKey", strings.NewReader("grpc-body"))
			rec := httptest.NewRecorder()
			rp.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type recordingTransport struct {
	closeIdleConnectionsCalls int
}

func (*recordingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
	}, nil
}

func (t *recordingTransport) CloseIdleConnections() {
	t.closeIdleConnectionsCalls++
}

func TestReverseProxyRouteClosesIdleConnectionsOnCancel(t *testing.T) {
	testTask := task.GetTestTask(t)
	entrypoint.SetCtx(testTask, newTestEntrypoint())

	base := &route.Route{
		Alias:       "test-close-idle",
		Scheme:      route.SchemeHTTP,
		Host:        "example.com",
		Port:        route.Port{Proxy: 80},
		HealthCheck: health.HealthCheckConfig{Disable: true},
	}
	require.NoError(t, base.Validate())

	routeImpl, err := routeimpl.NewReverseProxyRoute(base)
	require.NoError(t, err)

	transport := &recordingTransport{}
	routeImpl.ReverseProxy().Transport = transport

	require.NoError(t, routeImpl.Start(testTask))
	routeImpl.FinishAndWait("test done")

	require.Equal(t, 1, transport.closeIdleConnectionsCalls)
}
