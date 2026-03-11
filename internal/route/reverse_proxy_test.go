package route

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/homepage"
	route "github.com/yusing/godoxy/internal/route/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/task"
)

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
	httpRoutes     *testPool[types.HTTPRoute]
	streamRoutes   *testPool[types.StreamRoute]
	excludedRoutes *testPool[types.Route]
}

func newTestEntrypoint() *testEntrypoint {
	return &testEntrypoint{
		httpRoutes:     newTestPool[types.HTTPRoute](),
		streamRoutes:   newTestPool[types.StreamRoute](),
		excludedRoutes: newTestPool[types.Route](),
	}
}

func (ep *testEntrypoint) SupportProxyProtocol() bool { return false }
func (ep *testEntrypoint) DisablePoolsLog(bool)       {}

func (ep *testEntrypoint) GetRoute(alias string) (types.Route, bool) {
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

func (ep *testEntrypoint) StartAddRoute(r types.Route) error {
	if r.ShouldExclude() {
		ep.excludedRoutes.Add(r)
		return nil
	}
	switch rt := r.(type) {
	case types.HTTPRoute:
		ep.httpRoutes.Add(rt)
		return nil
	case types.StreamRoute:
		ep.streamRoutes.Add(rt)
		return nil
	default:
		return fmt.Errorf("unknown route type: %T", r)
	}
}

func (ep *testEntrypoint) IterRoutes(yield func(r types.Route) bool) {
	ep.httpRoutes.Iter(func(_ string, r types.HTTPRoute) bool {
		return yield(r)
	})
	ep.streamRoutes.Iter(func(_ string, r types.StreamRoute) bool {
		return yield(r)
	})
	ep.excludedRoutes.Iter(func(_ string, r types.Route) bool {
		return yield(r)
	})
}

func (ep *testEntrypoint) NumRoutes() int {
	return ep.httpRoutes.Size() + ep.streamRoutes.Size() + ep.excludedRoutes.Size()
}

func (ep *testEntrypoint) RoutesByProvider() map[string][]types.Route {
	return map[string][]types.Route{}
}

func (ep *testEntrypoint) HTTPRoutes() entrypoint.PoolLike[types.HTTPRoute] {
	return ep.httpRoutes
}

func (ep *testEntrypoint) StreamRoutes() entrypoint.PoolLike[types.StreamRoute] {
	return ep.streamRoutes
}

func (ep *testEntrypoint) ExcludedRoutes() entrypoint.RWPoolLike[types.Route] {
	return ep.excludedRoutes
}

func (ep *testEntrypoint) GetHealthInfo() map[string]types.HealthInfo {
	return nil
}

func (ep *testEntrypoint) GetHealthInfoWithoutDetail() map[string]types.HealthInfoWithoutDetail {
	return nil
}

func (ep *testEntrypoint) GetHealthInfoSimple() map[string]types.HealthStatus {
	return nil
}

func TestReverseProxyRoute(t *testing.T) {
	t.Run("LinkToLoadBalancer", func(t *testing.T) {
		testTask := task.GetTestTask(t)
		entrypoint.SetCtx(testTask, newTestEntrypoint())

		cfg := Route{
			Alias:  "test",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   Port{Proxy: 80},
			LoadBalance: &types.LoadBalancerConfig{
				Link: "test",
			},
		}
		cfg1 := Route{
			Alias:  "test1",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   Port{Proxy: 80},
			LoadBalance: &types.LoadBalancerConfig{
				Link: "test",
			},
		}
		r, err := NewStartedTestRoute(t, &cfg)
		require.NoError(t, err)
		assert.NotNil(t, r)
		r2, err := NewStartedTestRoute(t, &cfg1)
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

		makeRoute := func(alias string, target *httptest.Server) *Route {
			t.Helper()

			targetURL, err := url.Parse(target.URL)
			require.NoError(t, err)

			host, portStr, err := net.SplitHostPort(targetURL.Host)
			require.NoError(t, err)
			port, err := strconv.Atoi(portStr)
			require.NoError(t, err)

			return &Route{
				Alias:  alias,
				Scheme: route.SchemeHTTP,
				Host:   host,
				Port:   Port{Proxy: port},
				Homepage: &homepage.ItemConfig{
					Show: true,
				},
				LoadBalance: &types.LoadBalancerConfig{
					Link: "lb-test",
				},
				HealthCheck: types.HealthCheckConfig{
					Path:     "/",
					Interval: 2 * time.Second,
					Timeout:  time.Second,
					UseGet:   true,
				},
			}
		}

		_, err := NewStartedTestRoute(t, makeRoute("lb-1", srv1))
		require.NoError(t, err)
		_, err = NewStartedTestRoute(t, makeRoute("lb-2", srv2))
		require.NoError(t, err)
		_, err = NewStartedTestRoute(t, makeRoute("lb-3", srv3))
		require.NoError(t, err)

		ep := entrypoint.FromCtx(testTask.Context())
		require.NotNil(t, ep)

		lbRoute, ok := ep.HTTPRoutes().Get("lb-test")
		require.True(t, ok)

		lb, ok := lbRoute.(*ReverseProxyRoute)
		require.True(t, ok)
		require.False(t, lb.ShouldExclude())
		require.NotNil(t, lb.loadBalancer)
		require.NotNil(t, lb.HealthMonitor())
		assert.Equal(t, route.SchemeNone, lb.Scheme)
		assert.Empty(t, lb.Host)
		assert.Zero(t, lb.Port.Proxy)
		assert.Equal(t, "3/3 servers are healthy", lb.HealthMonitor().Detail())
	})
}
