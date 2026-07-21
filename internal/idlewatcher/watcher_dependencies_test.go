package idlewatcher

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/homepage"
	idlewatchertypes "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	"github.com/yusing/godoxy/internal/loadbalancer"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/routing"
	godoxytypes "github.com/yusing/godoxy/internal/types"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/http/reverseproxy"
	"github.com/yusing/goutils/task"
)

func TestNewWatcherReloadClearsDependencies(t *testing.T) {
	w, parent, mainRoute, _ := newDependencyReloadTest(t, "clear-deps", []string{"old"})
	require.Len(t, w.dependsOn, 1)
	w.sendEvent(WakeEventError, "stale attempt", errors.New("stale wake error"))
	require.NotEmpty(t, w.events.Get())

	reloaded, err := NewWatcher(parent, mainRoute, idlewatcherTestConfig("clear-deps", nil))
	require.NoError(t, err)
	require.Same(t, w, reloaded)
	require.Empty(t, reloaded.dependsOn)
	require.Empty(t, reloaded.cfg.DependsOn)
	require.Empty(t, reloaded.events.Get())
}

func TestNewWatcherReloadReplacesDependencies(t *testing.T) {
	w, parent, mainRoute, _ := newDependencyReloadTest(t, "replace-deps", []string{"old"})
	require.Len(t, w.dependsOn, 1)

	reloaded, err := NewWatcher(parent, mainRoute, idlewatcherTestConfig("replace-deps", []string{"new"}))
	require.NoError(t, err)
	require.Same(t, w, reloaded)
	require.Len(t, reloaded.dependsOn, 1)
	require.Equal(t, "new-container", reloaded.dependsOn[0].cfg.ContainerName())
	require.Equal(t, []string{"new-container"}, reloaded.cfg.DependsOn)
}

func TestNewWatcherReloadKeepsDependenciesAfterResolutionError(t *testing.T) {
	w, parent, mainRoute, providerCloses := newDependencyReloadTest(t, "bad-deps", []string{"old"})
	require.Len(t, w.dependsOn, 1)

	reloaded, err := NewWatcher(parent, mainRoute, idlewatcherTestConfig("bad-deps", []string{"new:unsupported"}))
	require.Error(t, err)
	require.Nil(t, reloaded)
	require.Len(t, w.dependsOn, 1)
	require.Equal(t, "old-container", w.dependsOn[0].cfg.ContainerName())
	require.Equal(t, []string{"old-container"}, w.cfg.DependsOn)
	require.Zero(t, providerCloses.Load())

	watcherMapMu.RLock()
	_, newDepExists := watcherMap["new-id"]
	watcherMapMu.RUnlock()
	require.False(t, newDepExists)
}

func TestNewWatcherReloadDoesNotCreateDependenciesAfterResolutionError(t *testing.T) {
	w, parent, mainRoute, providerCloses := newDependencyReloadTest(t, "mixed-bad-deps", []string{"old"})
	require.Len(t, w.dependsOn, 1)

	reloaded, err := NewWatcher(parent, mainRoute, idlewatcherTestConfig("mixed-bad-deps", []string{"new", "bad:unsupported"}))
	require.Error(t, err)
	require.Nil(t, reloaded)
	require.Len(t, w.dependsOn, 1)
	require.Equal(t, "old-container", w.dependsOn[0].cfg.ContainerName())
	require.Equal(t, []string{"old-container"}, w.cfg.DependsOn)
	require.Zero(t, providerCloses.Load())

	watcherMapMu.RLock()
	_, newDepExists := watcherMap["new-id"]
	watcherMapMu.RUnlock()
	require.False(t, newDepExists)
}

func TestNewWatcherReloadKeepsDependenciesAfterProviderError(t *testing.T) {
	w, parent, mainRoute, providerCloses := newDependencyReloadTest(t, "provider-error", []string{"old"})
	require.Len(t, w.dependsOn, 1)

	newDockerProvider = func(godoxytypes.DockerProviderConfig, string) (idlewatchertypes.Provider, error) {
		return nil, errors.New("provider unavailable")
	}

	reloaded, err := NewWatcher(parent, mainRoute, idlewatcherTestConfig("provider-error", []string{"new"}))
	require.ErrorContains(t, err, "provider unavailable")
	require.Nil(t, reloaded)
	require.Len(t, w.dependsOn, 1)
	require.Equal(t, "old-container", w.dependsOn[0].cfg.ContainerName())
	require.Equal(t, []string{"old-container"}, w.cfg.DependsOn)
	require.Zero(t, providerCloses.Load())

	watcherMapMu.RLock()
	_, newDepExists := watcherMap["new-id"]
	watcherMapMu.RUnlock()
	require.False(t, newDepExists)
}

func newDependencyReloadTest(t *testing.T, id string, dependsOn []string) (*Watcher, *task.Task, *idlewatcherTestRoute, *atomic.Int64) {
	t.Helper()

	oldDockerProvider := newDockerProvider
	oldProxmoxProvider := newProxmoxProvider
	providerCloses := &atomic.Int64{}
	newDockerProvider = func(godoxytypes.DockerProviderConfig, string) (idlewatchertypes.Provider, error) {
		return &idlewatcherTestRuntimeProvider{closes: providerCloses}, nil
	}
	newProxmoxProvider = func(context.Context, string, uint64) (idlewatchertypes.Provider, error) {
		return &idlewatcherTestRuntimeProvider{closes: providerCloses}, nil
	}
	t.Cleanup(func() {
		newDockerProvider = oldDockerProvider
		newProxmoxProvider = oldProxmoxProvider

		watcherMapMu.Lock()
		clear(watcherMap)
		watcherMapMu.Unlock()
	})

	parent := task.GetTestTask(t).Subtask("idlewatcher_"+id, true)
	t.Cleanup(func() {
		parent.FinishAndWait(nil)
	})

	routeProvider := &idlewatcherTestRouteProvider{routes: make(map[string]routing.Route)}
	mainRoute := newIdlewatcherTestRoute("main", routeProvider, idlewatcherTestConfig(id, dependsOn))
	routeProvider.routes["old"] = newIdlewatcherTestRoute("old", routeProvider, nil)
	routeProvider.routes["new"] = newIdlewatcherTestRoute("new", routeProvider, nil)

	w, err := NewWatcher(parent, mainRoute, mainRoute.cfg)
	require.NoError(t, err)
	return w, parent, mainRoute, providerCloses
}

func idlewatcherTestConfig(id string, dependsOn []string) *idlewatchertypes.Config {
	return &idlewatchertypes.Config{
		IdlewatcherProviderConfig: idlewatchertypes.ProviderConfig{
			Docker: &idlewatchertypes.DockerConfig{
				ContainerID:   id,
				ContainerName: id + "-container",
			},
		},
		IdlewatcherConfigBase: idlewatchertypes.ConfigBase{
			IdleTimeout: time.Hour,
			WakeTimeout: time.Second,
			StopTimeout: time.Second,
		},
		DependsOn: dependsOn,
	}
}

type idlewatcherTestRuntimeProvider struct {
	closes *atomic.Int64
}

func (*idlewatcherTestRuntimeProvider) ContainerPause(context.Context) error   { return nil }
func (*idlewatcherTestRuntimeProvider) ContainerUnpause(context.Context) error { return nil }
func (*idlewatcherTestRuntimeProvider) ContainerStart(context.Context) error   { return nil }
func (*idlewatcherTestRuntimeProvider) ContainerStop(context.Context, idlewatchertypes.Signal, int) error {
	return nil
}
func (*idlewatcherTestRuntimeProvider) ContainerKill(context.Context, idlewatchertypes.Signal) error {
	return nil
}
func (*idlewatcherTestRuntimeProvider) ContainerStatus(context.Context) (idlewatchertypes.ContainerStatus, error) {
	return idlewatchertypes.ContainerStatusStopped, nil
}
func (*idlewatcherTestRuntimeProvider) Watch(ctx context.Context) (<-chan watcherEvents.Event, <-chan error) {
	eventCh := make(chan watcherEvents.Event)
	errCh := make(chan error)
	go func() {
		<-ctx.Done()
		close(eventCh)
		close(errCh)
	}()
	return eventCh, errCh
}
func (p *idlewatcherTestRuntimeProvider) Close() {
	p.closes.Add(1)
}

type idlewatcherTestRouteProvider struct {
	routes map[string]routing.Route
}

func (p *idlewatcherTestRouteProvider) Start(task.Parent) error { return nil }
func (p *idlewatcherTestRouteProvider) LoadRoutes() error       { return nil }
func (p *idlewatcherTestRouteProvider) GetRoute(alias string) (routing.Route, bool) {
	r, ok := p.routes[alias]
	return r, ok
}
func (p *idlewatcherTestRouteProvider) IterRoutes(yield func(string, routing.Route) bool) {
	for alias, r := range p.routes {
		if !yield(alias, r) {
			return
		}
	}
}
func (p *idlewatcherTestRouteProvider) NumRoutes() int { return len(p.routes) }
func (p *idlewatcherTestRouteProvider) FindService(_, service string) (routing.Route, bool) {
	return p.GetRoute(service)
}
func (p *idlewatcherTestRouteProvider) Statistics() routing.ProviderStats {
	return routing.ProviderStats{Type: routing.ProviderTypeDocker}
}
func (p *idlewatcherTestRouteProvider) GetType() routing.ProviderType {
	return routing.ProviderTypeDocker
}
func (p *idlewatcherTestRouteProvider) ShortName() string { return "test" }
func (p *idlewatcherTestRouteProvider) String() string    { return "test" }

type idlewatcherTestRoute struct {
	name      string
	cfg       *idlewatchertypes.Config
	provider  routing.Provider
	started   chan struct{}
	targetURL *nettypes.URL
	container *docker.Container
	healthMon health.HealthMonitor
}

func newIdlewatcherTestRoute(name string, provider routing.Provider, cfg *idlewatchertypes.Config) *idlewatcherTestRoute {
	started := make(chan struct{})
	close(started)
	targetURL := nettypes.NewURL(&url.URL{Scheme: "http", Host: name + ".test"})

	return &idlewatcherTestRoute{
		name:      name,
		cfg:       cfg,
		provider:  provider,
		started:   started,
		targetURL: targetURL,
		container: &docker.Container{
			ContainerID:   name + "-id",
			ContainerName: name + "-container",
			Labels: map[string]string{
				"com.docker.compose.project": "idlewatcher-test",
			},
		},
	}
}

func (r *idlewatcherTestRoute) Start(task.Parent) error             { return nil }
func (r *idlewatcherTestRoute) Task() *task.Task                    { return nil }
func (r *idlewatcherTestRoute) Finish(any)                          {}
func (r *idlewatcherTestRoute) Key() string                         { return r.name }
func (r *idlewatcherTestRoute) Name() string                        { return r.name }
func (r *idlewatcherTestRoute) ProviderName() string                { return "test" }
func (r *idlewatcherTestRoute) GetProvider() routing.Provider       { return r.provider }
func (r *idlewatcherTestRoute) ListenURL() *nettypes.URL            { return nil }
func (r *idlewatcherTestRoute) TargetURL() *nettypes.URL            { return r.targetURL }
func (r *idlewatcherTestRoute) HealthMonitor() health.HealthMonitor { return r.healthMon }
func (r *idlewatcherTestRoute) SetHealthMonitor(m health.HealthMonitor) {
	r.healthMon = m
}
func (r *idlewatcherTestRoute) References() []string                        { return nil }
func (r *idlewatcherTestRoute) ShouldExclude() bool                         { return false }
func (r *idlewatcherTestRoute) Started() <-chan struct{}                    { return r.started }
func (r *idlewatcherTestRoute) IdlewatcherConfig() *idlewatchertypes.Config { return r.cfg }
func (r *idlewatcherTestRoute) HealthCheckConfig() health.HealthCheckConfig {
	return health.HealthCheckConfig{}
}
func (r *idlewatcherTestRoute) LoadBalanceConfig() *loadbalancer.Config { return nil }
func (r *idlewatcherTestRoute) HomepageItem() homepage.Item             { return homepage.Item{} }
func (r *idlewatcherTestRoute) DisplayName() string                     { return r.name }
func (r *idlewatcherTestRoute) ContainerInfo() *docker.Container        { return r.container }
func (r *idlewatcherTestRoute) InboundMTLSProfileRef() string           { return "" }
func (r *idlewatcherTestRoute) RouteMiddlewares() map[string]godoxytypes.LabelMap {
	return nil
}
func (r *idlewatcherTestRoute) GetAgent() *agentpool.Agent { return nil }
func (r *idlewatcherTestRoute) IsDocker() bool             { return true }
func (r *idlewatcherTestRoute) IsAgent() bool              { return false }
func (r *idlewatcherTestRoute) UseLoadBalance() bool       { return false }
func (r *idlewatcherTestRoute) UseIdleWatcher() bool       { return r.cfg != nil }
func (r *idlewatcherTestRoute) UseHealthCheck() bool       { return false }
func (r *idlewatcherTestRoute) UseAccessLog() bool         { return false }
func (r *idlewatcherTestRoute) ReverseProxy() *reverseproxy.ReverseProxy {
	return nil
}
func (r *idlewatcherTestRoute) ServeHTTP(http.ResponseWriter, *http.Request) {}
func (r *idlewatcherTestRoute) MarshalZerologObject(*zerolog.Event)          {}
