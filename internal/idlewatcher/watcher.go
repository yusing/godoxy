package idlewatcher

import (
	"context"
	"errors"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/idlewatcher/provider"
	idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/task"
	U "github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/atomic"
	"github.com/yusing/go-proxy/internal/watcher/events"
	"github.com/yusing/go-proxy/internal/watcher/health"
	"github.com/yusing/go-proxy/internal/watcher/health/monitor"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

type (
	routeHelper struct {
		route  routes.Route
		rp     *reverseproxy.ReverseProxy
		stream nettypes.Stream
		hc     health.HealthChecker
	}

	containerState struct {
		status idlewatcher.ContainerStatus
		ready  bool
		err    error
	}

	Watcher struct {
		_ U.NoCopy
		routeHelper

		l zerolog.Logger

		cfg *idlewatcher.Config

		provider idlewatcher.Provider

		state     atomic.Value[*containerState]
		lastReset atomic.Value[time.Time]

		idleTicker *time.Ticker
		task       *task.Task

		dependsOn []*dependency
	}

	dependency struct {
		*Watcher
		waitHealthy bool
	}

	StopCallback func() error
)

const ContextKey = "idlewatcher.watcher"

var (
	watcherMap   = make(map[string]*Watcher)
	watcherMapMu sync.RWMutex
	singleFlight singleflight.Group
)

const (
	idleWakerCheckInterval = 100 * time.Millisecond
	idleWakerCheckTimeout  = time.Second
)

var dummyHealthCheckConfig = &health.HealthCheckConfig{
	Interval: idleWakerCheckInterval,
	Timeout:  idleWakerCheckTimeout,
}

var (
	causeReload           = gperr.New("reloaded")            //nolint:errname
	causeContainerDestroy = gperr.New("container destroyed") //nolint:errname
)

const reqTimeout = 3 * time.Second

// prevents dependencies from being stopped automatically.
const neverTick = time.Duration(1<<63 - 1)

// TODO: fix stream type.
func NewWatcher(parent task.Parent, r routes.Route, cfg *idlewatcher.Config) (*Watcher, error) {
	key := cfg.Key()

	watcherMapMu.RLock()
	// if the watcher already exists, finish it
	w, exists := watcherMap[key]
	watcherMapMu.RUnlock()

	if exists {
		if len(cfg.DependsOn) > 0 {
			w.cfg.DependsOn = cfg.DependsOn
		}
		if cfg.IdleTimeout > 0 {
			w.cfg.IdlewatcherConfig = cfg.IdlewatcherConfig
		}
		cfg = w.cfg
		w.resetIdleTimer()
	} else {
		w = &Watcher{
			idleTicker: time.NewTicker(cfg.IdleTimeout),
			cfg:        cfg,
			routeHelper: routeHelper{
				hc: monitor.NewMonitor(r),
			},
			dependsOn: make([]*dependency, 0, len(cfg.DependsOn)),
		}
	}

	depErrors := gperr.NewBuilder()
	for i, dep := range cfg.DependsOn {
		depSegments := strings.Split(dep, ":")
		dep = depSegments[0]
		if dep == "" { // empty dependency (likely stopped container), skip; it will be removed by dedupDependencies()
			continue
		}
		cfg.DependsOn[i] = dep
		waitHealthy := false
		if len(depSegments) > 1 { // likely from `com.docker.compose.depends_on` label
			switch depSegments[1] {
			case "service_started":
			case "service_healthy":
				waitHealthy = true
			// case "service_completed_successfully":
			default:
				depErrors.Addf("dependency %q has unsupported condition %q", dep, depSegments[1])
				continue
			}
		}

		cont := r.ContainerInfo()

		var depRoute routes.Route
		var ok bool

		// try to find the dependency in the same provider and the same docker compose project first
		if cont != nil {
			depRoute, ok = r.GetProvider().FindService(cont.DockerComposeProject(), dep)
		}

		if !ok {
			depRoute, ok = routes.Get(dep)
			if !ok {
				depErrors.Addf("dependency %q not found", dep)
				continue
			}
		}

		if depRoute == r {
			depErrors.Addf("dependency %q cannot have itself as a dependency (same route)", dep)
			continue
		}

		// wait for the dependency to be started
		<-depRoute.Started()

		if waitHealthy && !depRoute.UseHealthCheck() {
			depErrors.Addf("dependency %q has service_healthy condition but has healthcheck disabled", dep)
			continue
		}

		depCfg := depRoute.IdlewatcherConfig()
		if depCfg == nil {
			depCfg = new(idlewatcher.Config)
			depCfg.IdlewatcherConfig = cfg.IdlewatcherConfig
			depCfg.IdleTimeout = neverTick // disable auto sleep for dependencies
		} else if depCfg.IdleTimeout > 0 {
			depErrors.Addf("dependency %q has positive idle timeout %s", dep, depCfg.IdleTimeout)
			continue
		}

		if depCfg.Docker == nil && depCfg.Proxmox == nil {
			depCont := depRoute.ContainerInfo()
			if depCont != nil {
				depCfg.Docker = &idlewatcher.DockerConfig{
					DockerHost:    depCont.DockerHost,
					ContainerID:   depCont.ContainerID,
					ContainerName: depCont.ContainerName,
				}
				depCfg.DependsOn = depCont.Dependencies()
			} else {
				depErrors.Addf("dependency %q has no idlewatcher config but is not a docker container", dep)
				continue
			}
		}

		if depCfg.Key() == cfg.Key() {
			depErrors.Addf("dependency %q cannot have itself as a dependency (same container)", dep)
			continue
		}

		depCfg.IdleTimeout = neverTick // disable auto sleep for dependencies

		depWatcher, err := NewWatcher(parent, depRoute, depCfg)
		if err != nil {
			depErrors.Add(err)
			continue
		}
		w.dependsOn = append(w.dependsOn, &dependency{
			Watcher:     depWatcher,
			waitHealthy: waitHealthy,
		})
	}

	if w.provider != nil { // it's a reload, close the old provider
		w.provider.Close()
	}

	if depErrors.HasError() {
		return nil, depErrors.Error()
	}

	if !exists {
		watcherMapMu.Lock()
		defer watcherMapMu.Unlock()
	}

	var p idlewatcher.Provider
	var err error
	var kind string
	switch {
	case cfg.Docker != nil:
		p, err = provider.NewDockerProvider(cfg.Docker.DockerHost, cfg.Docker.ContainerID)
		kind = "docker"
	default:
		p, err = provider.NewProxmoxProvider(cfg.Proxmox.Node, cfg.Proxmox.VMID)
		kind = "proxmox"
	}
	w.l = log.With().
		Str("kind", kind).
		Str("container", cfg.ContainerName()).
		Logger()

	if cfg.IdleTimeout != neverTick {
		w.l = w.l.With().Stringer("idle_timeout", cfg.IdleTimeout).Logger()
	}

	if err != nil {
		return nil, err
	}
	w.provider = p

	switch r := r.(type) {
	case routes.ReverseProxyRoute:
		w.rp = r.ReverseProxy()
	case routes.StreamRoute:
		w.stream = r.Stream()
	default:
		w.provider.Close()
		return nil, w.newWatcherError(gperr.Errorf("unexpected route type: %T", r))
	}
	w.route = r

	ctx, cancel := context.WithTimeout(parent.Context(), reqTimeout)
	defer cancel()
	status, err := w.provider.ContainerStatus(ctx)
	if err != nil {
		w.provider.Close()
		return nil, w.newWatcherError(err)
	}
	w.state.Store(&containerState{status: status})

	// when more providers are added, we need to add a new case here.
	switch p := w.provider.(type) { //nolint:gocritic
	case *provider.ProxmoxProvider:
		shutdownTimeout := max(time.Second, cfg.StopTimeout-idleWakerCheckTimeout)
		err = p.LXCSetShutdownTimeout(ctx, cfg.Proxmox.VMID, shutdownTimeout)
		if err != nil {
			w.l.Warn().Err(err).Msg("failed to set shutdown timeout")
		}
	}

	if !exists {
		w.task = parent.Subtask("idlewatcher."+r.Name(), true)
		watcherMap[key] = w

		go func() {
			cause := w.watchUntilDestroy()
			if errors.Is(cause, causeContainerDestroy) || errors.Is(cause, task.ErrProgramExiting) {
				watcherMapMu.Lock()
				delete(watcherMap, key)
				watcherMapMu.Unlock()
				w.l.Info().Msg("idlewatcher stopped")
			} else if !errors.Is(cause, causeReload) {
				gperr.LogError("idlewatcher stopped unexpectedly", cause, &w.l)
			}

			w.idleTicker.Stop()
			w.provider.Close()
			w.task.Finish(cause)
		}()
	}

	hcCfg := w.hc.Config()
	hcCfg.BaseContext = func() context.Context {
		return w.task.Context()
	}
	hcCfg.Timeout = cfg.WakeTimeout

	w.dedupDependencies()

	r.SetHealthMonitor(w)

	w.l = w.l.With().Strs("deps", cfg.DependsOn).Logger()
	if exists {
		w.l.Debug().Msg("idlewatcher reloaded")
	} else {
		w.l.Info().Msg("idlewatcher started")
	}
	return w, nil
}

func (w *Watcher) Key() string {
	return w.cfg.Key()
}

// Wake wakes the container.
//
// It will cancel as soon as the either of the passed in context or the watcher is done.
//
// It uses singleflight to prevent multiple wake calls at the same time.
//
// It will wake the dependencies first, and then wake itself.
// If the container is already running, it will do nothing.
// If the container is not running, it will start it.
// If the container is paused, it will unpause it.
// If the container is stopped, it will do nothing.
func (w *Watcher) Wake(ctx context.Context) error {
	// wake dependencies first.
	if err := w.wakeDependencies(ctx); err != nil {
		return w.newWatcherError(err)
	}

	// wake itself.
	// use container name instead of Key() here as the container id will change on restart (docker).
	_, err, _ := singleFlight.Do(w.cfg.ContainerName(), func() (any, error) {
		return nil, w.wakeIfStopped(ctx)
	})
	if err != nil {
		return w.newWatcherError(err)
	}

	return nil
}

func (w *Watcher) wakeDependencies(ctx context.Context) error {
	if len(w.dependsOn) == 0 {
		return nil
	}

	errs := errgroup.Group{}
	for _, dep := range w.dependsOn {
		errs.Go(func() error {
			if err := dep.Wake(ctx); err != nil {
				return err
			}
			if dep.waitHealthy {
				for {
					select {
					case <-ctx.Done():
						return w.newDepError("wait_healthy", dep, context.Cause(ctx))
					default:
						if h, err := dep.hc.CheckHealth(); err != nil {
							return err
						} else if h.Healthy {
							return nil
						}
						time.Sleep(idleWakerCheckInterval)
					}
				}
			}
			return nil
		})
	}
	return errs.Wait()
}

func (w *Watcher) wakeIfStopped(ctx context.Context) error {
	state := w.state.Load()
	if state.status == idlewatcher.ContainerStatusRunning {
		w.l.Debug().Msg("container is already running")
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, w.cfg.WakeTimeout)
	defer cancel()
	switch state.status {
	case idlewatcher.ContainerStatusStopped:
		w.l.Info().Msg("starting container")
		return w.provider.ContainerStart(ctx)
	case idlewatcher.ContainerStatusPaused:
		w.l.Info().Msg("unpausing container")
		return w.provider.ContainerUnpause(ctx)
	default:
		return gperr.Errorf("unexpected container status: %s", state.status)
	}
}

func (w *Watcher) stopDependencies() error {
	if len(w.dependsOn) == 0 {
		return nil
	}

	errs := errgroup.Group{}
	for _, dep := range w.dependsOn {
		errs.Go(dep.stopByMethod)
	}
	return errs.Wait()
}

func (w *Watcher) stopByMethod() error {
	// no need singleflight here because it will only be called once every tick.

	// if the container is not running, skip and stop dependencies.
	if !w.running() {
		if err := w.stopDependencies(); err != nil {
			return w.newWatcherError(err)
		}
		return nil
	}

	cfg := w.cfg
	ctx, cancel := context.WithTimeout(context.Background(), cfg.StopTimeout)
	defer cancel()

	// stop itself first.
	var err error
	switch cfg.StopMethod {
	case idlewatcher.StopMethodPause:
		err = w.provider.ContainerPause(ctx)
	case idlewatcher.StopMethodStop:
		err = w.provider.ContainerStop(ctx, cfg.StopSignal, int(cfg.StopTimeout.Seconds()))
	case idlewatcher.StopMethodKill:
		err = w.provider.ContainerKill(ctx, cfg.StopSignal)
	default:
		err = w.newWatcherError(gperr.Errorf("unexpected stop method: %q", cfg.StopMethod))
	}

	if err != nil {
		return w.newWatcherError(err)
	}

	w.l.Info().Msg("container stopped")

	// then stop dependencies.
	if err := w.stopDependencies(); err != nil {
		return w.newWatcherError(err)
	}
	return nil
}

func (w *Watcher) resetIdleTimer() {
	w.idleTicker.Reset(w.cfg.IdleTimeout)
	w.lastReset.Store(time.Now())
}

func (w *Watcher) expires() time.Time {
	if !w.running() || w.cfg.IdleTimeout <= 0 {
		return time.Time{}
	}
	return w.lastReset.Load().Add(w.cfg.IdleTimeout)
}

// watchUntilDestroy waits for the container to be created, started, or unpaused,
// and then reset the idle timer.
//
// When the container is stopped, paused,
// or killed, the idle timer is stopped and the ContainerRunning flag is set to false.
//
// When the idle timer fires, the container is stopped according to the
// stop method.
//
// it exits only if the context is canceled, the container is destroyed,
// errors occurred on docker client, or route provider died (mainly caused by config reload).
func (w *Watcher) watchUntilDestroy() (returnCause error) {
	eventCh, errCh := w.provider.Watch(w.Task().Context())

	for {
		select {
		case <-w.task.Context().Done():
			return gperr.Wrap(w.task.FinishCause())
		case err := <-errCh:
			gperr.LogError("watcher error", err, &w.l)
		case e := <-eventCh:
			w.l.Debug().Stringer("action", e.Action).Msg("state changed")
			switch e.Action {
			case events.ActionContainerDestroy:
				return causeContainerDestroy
			case events.ActionForceReload:
				continue
			}
			w.resetIdleTimer()
			switch {
			case e.Action.IsContainerStart(): // create / start / unpause
				w.setStarting()
				w.l.Info().Msg("awaken")
			case e.Action.IsContainerStop(): // stop / kill / die
				w.setNapping(idlewatcher.ContainerStatusStopped)
				w.idleTicker.Stop()
			case e.Action.IsContainerPause(): // pause
				w.setNapping(idlewatcher.ContainerStatusPaused)
				w.idleTicker.Stop()
			default:
				w.l.Debug().Stringer("action", e.Action).Msg("unexpected container action")
			}
		case <-w.idleTicker.C:
			w.idleTicker.Stop()
			if w.running() {
				err := w.stopByMethod()
				switch {
				case errors.Is(err, context.Canceled):
					continue
				case err != nil:
					if errors.Is(err, context.DeadlineExceeded) {
						err = errors.New("timeout waiting for container to stop, please set a higher value for `stop_timeout`")
					}
					w.l.Err(err).Msgf("container stop with method %q failed", w.cfg.StopMethod)
				default:
					w.l.Info().Msg("idle timeout")
				}
			}
		}
	}
}

func (w *Watcher) dedupDependencies() {
	// remove from dependencies if the dependency is also a dependency of another dependency, or have duplicates.
	deps := w.dependencies()
	for _, dep := range w.dependsOn {
		depdeps := dep.dependencies()
		for depdep := range depdeps {
			delete(deps, depdep)
		}
	}
	newDepOn := make([]string, 0, len(deps))
	newDeps := make([]*dependency, 0, len(deps))
	for _, dep := range deps {
		newDepOn = append(newDepOn, dep.cfg.ContainerName())
		newDeps = append(newDeps, dep)
	}
	w.cfg.DependsOn = newDepOn
	w.dependsOn = newDeps
}

func (w *Watcher) dependencies() map[string]*dependency {
	deps := make(map[string]*dependency)
	for _, dep := range w.dependsOn {
		deps[dep.Key()] = dep
		maps.Copy(deps, dep.dependencies())
	}
	return deps
}
