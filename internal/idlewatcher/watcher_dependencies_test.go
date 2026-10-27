package idlewatcher

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/docker"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint"
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

func TestStopSharedDependencies(t *testing.T) {
	for _, method := range []idlewatchertypes.ContainerStopMethod{
		idlewatchertypes.ContainerStopMethodStop,
		idlewatchertypes.ContainerStopMethodPause,
	} {
		t.Run(string(method), func(t *testing.T) {
			watchers := make([]*Watcher, 4)
			providers := make([]*dependencyLifecycleProvider, 4)
			for i := range watchers {
				w, p := newDependencyLifecycleWatcher(t)
				providers[i] = p
				w.cfg.StopMethod = method
				watchers[i] = w
			}
			first, second, shared, leaf := watchers[0], watchers[1], watchers[2], watchers[3]
			first.setDependencies([]*dependency{{Watcher: shared}})
			second.setDependencies([]*dependency{{Watcher: shared}})
			shared.setDependencies([]*dependency{{Watcher: leaf}})

			require.NoError(t, first.stopByMethod())
			require.EqualValues(t, 1, providers[0].stops.Load())
			require.Zero(t, providers[1].stops.Load())
			require.Zero(t, providers[2].stops.Load(), "shared dependency stopped while another dependent runs")
			require.Zero(t, providers[3].stops.Load(), "shared dependency's own dependencies must stay running")

			// No provider events are delivered: successful stops must release
			// dependencies without waiting for asynchronous event processing.
			require.NoError(t, second.stopByMethod())
			for i, w := range watchers {
				require.EqualValues(t, 1, providers[i].stops.Load())
				status := idlewatchertypes.ContainerStatusStopped
				if method == idlewatchertypes.ContainerStopMethodPause {
					status = idlewatchertypes.ContainerStatusPaused
				}
				require.Equal(t, status, w.state.Load().status)
			}

			require.NoError(t, first.Wake(t.Context()))
			for _, i := range []int{0, 2, 3} {
				require.True(t, watchers[i].running(), "wake must restore the transitive dependency chain")
				require.EqualValues(t, 1, providers[i].starts.Load())
			}
			require.False(t, second.running(), "an unrelated sleeping route must stay asleep")
		})
	}
}

func TestWakeWaitsForDependencyStop(t *testing.T) {
	for _, target := range []string{"parent", "dependency"} {
		t.Run(target, func(t *testing.T) {
			root, _ := newDependencyLifecycleWatcher(t)
			dep, p := newDependencyLifecycleWatcher(t)
			root.setDependencies([]*dependency{{Watcher: dep}})
			events := make(chan watcherEvents.Event)
			p.events = events
			entered, release := make(chan struct{}), make(chan struct{})
			p.stop = func(ctx context.Context) error {
				events <- watcherEvents.Event{Action: watcherEvents.ActionContainerStop}
				close(entered)
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return context.Cause(ctx)
				}
			}
			watched := make(chan error, 1)
			go func() { watched <- dep.watchUntilDestroy() }()
			t.Cleanup(func() { dep.task.Finish(nil); <-watched })
			defer close(release)
			stopped := make(chan error, 1)
			go func() { stopped <- root.stopByMethod() }()
			requireChannelClosed(t, entered, "dependency stop did not start")

			w := root
			if target == "dependency" {
				w = dep
			}
			woke := w.startWake()
			select {
			case result := <-woke:
				t.Fatalf("wake completed before the dependency stop: %v", result.Err)
			case <-time.After(20 * time.Millisecond):
			}
			release <- struct{}{}
			require.NoError(t, <-stopped)
			require.NoError(t, w.waitWake(t.Context(), woke))
			require.True(t, w.running())
			require.True(t, dep.running(), "late stop response must not overwrite a newer wake")
			require.EqualValues(t, 1, p.starts.Load())
		})
	}
}

func TestKillWaitsForRuntimeStop(t *testing.T) {
	for _, stopped := range []bool{false, true} {
		t.Run(fmt.Sprint(stopped), func(t *testing.T) {
			w, p := newDependencyLifecycleWatcher(t)
			dep, dp := newDependencyLifecycleWatcher(t)
			w.setDependencies([]*dependency{{Watcher: dep}})
			w.cfg.StopMethod = idlewatchertypes.ContainerStopMethodKill
			w.cfg.StopSignal = "SIGHUP"
			w.cfg.StopTimeout = 20 * time.Millisecond
			p.killStops = stopped
			err := w.stopByMethod()
			if stopped {
				require.NoError(t, err)
				require.False(t, w.running())
				require.EqualValues(t, 1, dp.stops.Load())
			} else {
				require.ErrorContains(t, err, "timeout")
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.True(t, w.running())
				require.Zero(t, dp.stops.Load())
			}
		})
	}
}

func TestWakeDeadlineDuringStop(t *testing.T) {
	w, p := newDependencyLifecycleWatcher(t)
	history := attachIdleActivityHistory(t, w)
	w.cfg.WakeTimeout = 20 * time.Millisecond
	entered, release := make(chan struct{}), make(chan struct{})
	p.stop = func(ctx context.Context) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	stopped := make(chan error, 1)
	go func() { stopped <- w.stopByMethod() }()
	requireChannelClosed(t, entered, "stop did not start")
	defer func() { close(release); <-stopped }()
	select {
	case result := <-w.startWake():
		require.ErrorIs(t, result.Err, context.DeadlineExceeded)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("wake deadline did not interrupt lifecycle wait")
	}
	require.NotNil(t, idleActivityAction(history, IdleEventActionError))
}

func TestDelayedStopEventKeepsSharedDependency(t *testing.T) {
	first, p := newDependencyLifecycleWatcher(t)
	second, _ := newDependencyLifecycleWatcher(t)
	shared, sp := newDependencyLifecycleWatcher(t)
	first.setDependencies([]*dependency{{Watcher: shared}})
	second.setDependencies([]*dependency{{Watcher: shared}})
	events := make(chan watcherEvents.Event)
	p.events = events
	watched := make(chan error, 1)
	go func() { watched <- first.watchUntilDestroy() }()
	t.Cleanup(func() { first.task.Finish(nil); <-watched })
	require.NoError(t, first.stopByMethod())
	require.NoError(t, first.Wake(t.Context()))
	events <- watcherEvents.Event{Action: watcherEvents.ActionContainerStop}
	// Receipt of the next event establishes that the stop event was handled.
	events <- watcherEvents.Event{Action: watcherEvents.ActionForceReload}
	require.True(t, first.running())
	require.NoError(t, second.stopByMethod())
	require.Zero(t, sp.stops.Load(), "a delayed stop event released a live dependency")
}

func newDependencyLifecycleWatcher(t *testing.T) (*Watcher, *dependencyLifecycleProvider) {
	t.Helper()
	w := newTestWatcher(t)
	p := &dependencyLifecycleProvider{blockingStartProvider: &blockingStartProvider{
		started: make(chan context.Context, 1),
		release: make(chan struct{}),
	}}
	close(p.release)
	p.status.Store(idlewatchertypes.ContainerStatusRunning)
	w.provider.Store(p)
	w.cfg.StopMethod = idlewatchertypes.ContainerStopMethodStop
	w.cfg.StopTimeout = time.Second
	w.setReady()
	watcherMapMu.Lock()
	watcherMap[w.Key()] = w
	watcherMapMu.Unlock()
	t.Cleanup(func() {
		watcherMapMu.Lock()
		delete(watcherMap, w.Key())
		watcherMapMu.Unlock()
	})
	return w, p
}

type dependencyLifecycleProvider struct {
	*blockingStartProvider
	stops     atomic.Int64
	killStops bool
	status    atomic.Value
	stop      func(context.Context) error
	events    <-chan watcherEvents.Event
}

func (p *dependencyLifecycleProvider) ContainerStop(ctx context.Context, _ idlewatchertypes.Signal, _ int) error {
	p.stops.Add(1)
	p.status.Store(idlewatchertypes.ContainerStatusStopped)
	if p.stop != nil {
		return p.stop(ctx)
	}
	return nil
}

func (p *dependencyLifecycleProvider) ContainerKill(context.Context, idlewatchertypes.Signal) error {
	if p.killStops {
		p.status.Store(idlewatchertypes.ContainerStatusStopped)
	}
	return nil
}

func (p *dependencyLifecycleProvider) ContainerPause(context.Context) error {
	p.stops.Add(1)
	p.status.Store(idlewatchertypes.ContainerStatusPaused)
	return nil
}

func (p *dependencyLifecycleProvider) ContainerStart(ctx context.Context) error {
	if err := p.blockingStartProvider.ContainerStart(ctx); err != nil {
		return err
	}
	p.status.Store(idlewatchertypes.ContainerStatusRunning)
	return nil
}

func (p *dependencyLifecycleProvider) ContainerStatus(context.Context) (idlewatchertypes.ContainerStatus, error) {
	return p.status.Load().(idlewatchertypes.ContainerStatus), nil
}

func (p *dependencyLifecycleProvider) ContainerUnpause(ctx context.Context) error {
	return p.ContainerStart(ctx)
}

func (p *dependencyLifecycleProvider) Watch(context.Context) (<-chan watcherEvents.Event, <-chan error) {
	return p.events, nil
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

	newDockerProvider = func(context.Context, godoxytypes.DockerProviderConfig, string) (idlewatchertypes.Provider, error) {
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
	newDockerProvider = func(context.Context, godoxytypes.DockerProviderConfig, string) (idlewatchertypes.Provider, error) {
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
		Docker: &idlewatchertypes.DockerConfig{
			ContainerID:   id,
			ContainerName: id + "-container",
		},
		IdleTimeout: time.Hour,
		WakeTimeout: time.Second,
		StopTimeout: time.Second,
		DependsOn:   dependsOn,
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

func (p *idlewatcherTestRouteProvider) Activate(task.Parent) routing.ProviderActivation {
	return p.ActivationSnapshot()
}
func (p *idlewatcherTestRouteProvider) ActivationSnapshot() routing.ProviderActivation {
	return routing.ProviderActivation{Provider: p.String(), EventLoopReady: true}
}
func (p *idlewatcherTestRouteProvider) LoadRoutes(context.Context) error { return nil }
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

// A watcher that is both its own idle route and another route's dependency gets
// NewWatcher called a second time with a dependency-synthesized config. That
// config carries neverTick so the dependency never auto-sleeps, and adopting it
// would silently disable the route's own idle timeout.
func TestNewWatcherReloadIgnoresDependencyConfig(t *testing.T) {
	w, parent, mainRoute, _ := newDependencyReloadTest(t, "dep-clobber", nil)
	require.Equal(t, time.Hour, w.cfg.IdleTimeout)

	depCfg := idlewatcherTestConfig("dep-clobber", nil)
	depCfg.IdleTimeout = neverTick
	depCfg.WakeTimeout = 42 * time.Second
	depCfg.StopTimeout = 10 * time.Second
	depCfg.StopMethod = idlewatchertypes.ContainerStopMethodKill
	depCfg.StopSignal = "SIGKILL"
	want := w.cfg.IdlewatcherConfigBase

	reloaded, err := NewWatcher(parent, mainRoute, depCfg)
	require.NoError(t, err)
	require.Same(t, w, reloaded)
	require.Equal(t, time.Hour, reloaded.cfg.IdleTimeout, "route keeps its own idle timeout")
	require.Equal(t, want, reloaded.cfg.IdlewatcherConfigBase, "route keeps all of its own base config")
}

// A genuine reload must still be adopted.
func TestNewWatcherReloadAdoptsUpdatedConfig(t *testing.T) {
	w, parent, mainRoute, _ := newDependencyReloadTest(t, "adopt-cfg", nil)
	require.Equal(t, time.Hour, w.cfg.IdleTimeout)

	newCfg := idlewatcherTestConfig("adopt-cfg", nil)
	newCfg.IdleTimeout = 2 * time.Hour
	newCfg.WakeTimeout = 5 * time.Second
	newCfg.StopTimeout = 10 * time.Second
	newCfg.StopMethod = idlewatchertypes.ContainerStopMethodKill
	newCfg.StopSignal = "SIGKILL"

	reloaded, err := NewWatcher(parent, mainRoute, newCfg)
	require.NoError(t, err)
	require.Same(t, w, reloaded)
	require.Equal(t, 2*time.Hour, reloaded.cfg.IdleTimeout)
	require.Equal(t, newCfg.IdlewatcherConfigBase, reloaded.cfg.IdlewatcherConfigBase)
}

func TestNewWatcherReloadDependencyOnlyConfig(t *testing.T) {
	for _, tc := range []struct {
		name        string
		idleTimeout time.Duration
		adopt       bool
	}{
		{"refresh", neverTick, true},
		{"promote_to_route", 2 * time.Hour, true},
		{"zero_unchanged", 0, false},
		{"negative_unchanged", -1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, parent, mainRoute, _ := newDependencyReloadTest(t, "dep-only", []string{"old"})
			dep := w.dependsOn[0].Watcher
			require.Equal(t, neverTick, dep.cfg.IdleTimeout)
			want := dep.cfg.IdlewatcherConfigBase
			cfg := idlewatcherTestConfig("old-id", nil)
			cfg.IdleTimeout = tc.idleTimeout
			cfg.WakeTimeout = 42 * time.Second
			cfg.StopTimeout = 10 * time.Second
			cfg.StopMethod = idlewatchertypes.ContainerStopMethodKill
			cfg.StopSignal = "SIGKILL"
			if tc.adopt {
				want = cfg.IdlewatcherConfigBase
			}
			depRoute, ok := mainRoute.provider.GetRoute("old")
			require.True(t, ok)
			reloaded, err := NewWatcher(parent, depRoute, cfg)
			require.NoError(t, err)
			require.Same(t, dep, reloaded)
			require.Equal(t, want, reloaded.cfg.IdlewatcherConfigBase)
		})
	}
}

func TestNewWatcherReloadRefreshesDependencySettings(t *testing.T) {
	for _, tc := range []struct {
		name        string
		explicit    bool
		idleTimeout time.Duration
	}{
		{name: "inherited"},
		{name: "explicit_zero", explicit: true},
		{name: "explicit_negative", explicit: true, idleTimeout: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, parent, mainRoute, _ := newDependencyReloadTest(t, "parent-reload", []string{"old"})
			dep := w.dependsOn[0].Watcher
			require.Equal(t, neverTick, dep.cfg.IdleTimeout)
			cfg := idlewatcherTestConfig("parent-reload", []string{"old"})
			updated := idlewatchertypes.IdlewatcherConfigBase{
				IdleTimeout: neverTick,
				WakeTimeout: 42 * time.Second,
				StopTimeout: 10 * time.Second,
				StopMethod:  idlewatchertypes.ContainerStopMethodKill,
				StopSignal:  "SIGKILL",
			}
			if tc.explicit {
				depRoute, ok := mainRoute.provider.GetRoute("old")
				require.True(t, ok)
				depCfg := idlewatcherTestConfig("old-id", nil)
				depCfg.IdlewatcherConfigBase = updated
				depCfg.IdleTimeout = tc.idleTimeout
				depRoute.(*idlewatcherTestRoute).cfg = depCfg
			} else {
				cfg.IdlewatcherConfigBase = updated
				cfg.IdleTimeout = time.Hour
			}

			reloaded, err := NewWatcher(parent, mainRoute, cfg)
			require.NoError(t, err)
			require.Same(t, w, reloaded)
			require.Len(t, reloaded.dependsOn, 1)
			require.Same(t, dep, reloaded.dependsOn[0].Watcher)
			require.Equal(t, updated, dep.cfg.IdlewatcherConfigBase)
		})
	}
}

// staticHealthMonitor stands in for a route's health monitor.
type staticHealthMonitor struct {
	status health.HealthStatus
}

func (staticHealthMonitor) Start(task.Parent) error             { return nil }
func (staticHealthMonitor) Finish(any)                          {}
func (staticHealthMonitor) Task() *task.Task                    { return nil }
func (m staticHealthMonitor) String() string                    { return m.status.String() }
func (staticHealthMonitor) Name() string                        { return "static" }
func (m staticHealthMonitor) Status() health.HealthStatus       { return m.status }
func (staticHealthMonitor) Uptime() time.Duration               { return 0 }
func (staticHealthMonitor) Latency() time.Duration              { return 0 }
func (staticHealthMonitor) Detail() string                      { return "" }
func (staticHealthMonitor) MarshalJSON() ([]byte, error)        { return []byte(`"static"`), nil }
func (staticHealthMonitor) MarshalZerologObject(*zerolog.Event) {}

type alwaysOnDependencySetup struct {
	main, shared   *Watcher
	mainProvider   *dependencyLifecycleProvider
	sharedProvider *dependencyLifecycleProvider
	alwaysOn       *idlewatcherTestRoute
}

// newAlwaysOnDependencySetup wires a watcher that declares shared as its
// dependency, next to an always-on route that declares the same dependency
// through container labels. The always-on route has no idle timeout, so it
// never gets a watcher of its own: its labels are the only record of the edge.
func newAlwaysOnDependencySetup(t *testing.T, dependencyEntry string) *alwaysOnDependencySetup {
	t.Helper()

	main, mainProvider := newDependencyLifecycleWatcher(t)
	shared, sharedProvider := newDependencyLifecycleWatcher(t)
	main.setDependencies([]*dependency{{Watcher: shared}})

	ep := entrypoint.NewEntrypoint(task.GetTestTask(t), nil)
	entrypoint.SetCtx(task.GetTestTask(t), ep)

	routeProvider := &idlewatcherTestRouteProvider{routes: make(map[string]routing.Route)}
	depRoute := newIdlewatcherTestRoute("koito_backend", routeProvider, nil)
	depRoute.container.ContainerID = shared.Key()
	routeProvider.routes[depRoute.name] = depRoute

	alwaysOn := newIdlewatcherTestRoute("koito", routeProvider, nil)
	alwaysOn.container.ContainerID = "koito-container-id"
	alwaysOn.container.Labels = map[string]string{
		"com.docker.compose.project": "idlewatcher-test",
		"com.docker.compose.service": "koito",
		docker.LabelDependsOn:        dependencyEntry,
	}
	ep.ExcludedRoutes().Add(alwaysOn)

	return &alwaysOnDependencySetup{
		main:           main,
		shared:         shared,
		mainProvider:   mainProvider,
		sharedProvider: sharedProvider,
		alwaysOn:       alwaysOn,
	}
}

// Stopping an idle route must not stop a dependency an always-on route needs.
// An unhealthy route still needs its dependency up: the dependency may be what
// brings it back.
func TestStopSharedDependencyKeptForAlwaysOnRoute(t *testing.T) {
	for _, tc := range []struct {
		name   string
		entry  string
		health health.HealthStatus
	}{
		{name: "bare_service_name", entry: "koito_backend"},
		{name: "service_started", entry: "koito_backend:service_started"},
		{name: "service_healthy", entry: "koito_backend:service_healthy"}, // the condition must not break the lookup
		{name: "health_unknown", entry: "koito_backend", health: health.StatusUnknown},
		{name: "health_unhealthy", entry: "koito_backend", health: health.StatusUnhealthy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newAlwaysOnDependencySetup(t, tc.entry)
			s.alwaysOn.SetHealthMonitor(staticHealthMonitor{status: tc.health})

			require.NoError(t, s.main.stopByMethod())
			require.EqualValues(t, 1, s.mainProvider.stops.Load())
			require.Zero(t, s.sharedProvider.stops.Load(),
				"dependency stopped while an always-on route depends on it")
		})
	}
}

// A napping route does not pin its dependency awake: its next wake rebuilds the
// dependency chain.
func TestStopSharedDependencyWhenAlwaysOnRouteNaps(t *testing.T) {
	s := newAlwaysOnDependencySetup(t, "koito_backend")
	s.alwaysOn.SetHealthMonitor(staticHealthMonitor{status: health.StatusNapping})

	require.NoError(t, s.main.stopByMethod())
	require.EqualValues(t, 1, s.mainProvider.stops.Load())
	require.EqualValues(t, 1, s.sharedProvider.stops.Load(),
		"dependency must stop once the depending route is napping")
}
