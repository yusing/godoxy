package idlewatcher

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/ds/ordered"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/docker"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/health/monitor"
	"github.com/yusing/godoxy/internal/idlewatcher/provider"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/types"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/godoxy/internal/watcher/events"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/http/reverseproxy"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/synk"
	"github.com/yusing/goutils/task"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

type (
	routeHelper struct {
		route  types.Route
		rp     *reverseproxy.ReverseProxy
		stream nettypes.Stream
		hc     types.HealthChecker
	}

	containerState struct {
		status      idlewatcher.ContainerStatus
		ready       bool
		err         error
		startedAt   time.Time // when container started (for timeout detection)
		healthTries int       // number of failed health check attempts
	}

	Watcher struct {
		routeHelper

		l zerolog.Logger

		cfg *types.IdlewatcherConfig

		provider synk.Value[idlewatcher.Provider]

		state     synk.Value[*containerState]
		lastReset synk.Value[time.Time]

		idleTicker    *time.Ticker
		healthTicker  *time.Ticker
		readyNotifyCh chan struct{} // notifies when container becomes ready
		task          *task.Task

		// SSE event broadcasting, HTTP routes only
		eventChs       *xsync.Map[chan *WakeEvent, struct{}]
		eventHistory   []WakeEvent  // Global event history buffer
		eventHistoryMu sync.RWMutex // Mutex for event history

		// FIXME: missing dependencies
		dependsOn []*dependency
	}

	dependency struct {
		*Watcher

		waitHealthy bool
	}

	StopCallback func() error
)

const ContextKey = "idlewatcher.watcher"

var _ idlewatcher.Waker = (*Watcher)(nil)

var (
	watcherMap   = make(map[string]*Watcher)
	watcherMapMu sync.RWMutex
	singleFlight singleflight.Group
)

const (
	idleWakerCheckInterval = 200 * time.Millisecond
	idleWakerCheckTimeout  = time.Second
)

var (
	errCauseReload           = errors.New("reloaded")
	errCauseContainerDestroy = errors.New("container destroyed")
)

const reqTimeout = 3 * time.Second

// prevents dependencies from being stopped automatically.
const neverTick = time.Duration(1<<63 - 1)

func NewWatcher(parent task.Parent, r types.Route, cfg *types.IdlewatcherConfig) (*Watcher, error) {
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
			w.cfg.IdlewatcherConfigBase = cfg.IdlewatcherConfigBase
		}
		cfg = w.cfg
		w.resetIdleTimer()
		// Update health monitor URL with current route info on reload
		if targetURL := r.TargetURL(); targetURL != nil {
			w.hc.UpdateURL(&targetURL.URL)
		}
	} else {
		w = &Watcher{
			idleTicker:    time.NewTicker(cfg.IdleTimeout),
			healthTicker:  time.NewTicker(idleWakerCheckInterval),
			readyNotifyCh: make(chan struct{}, 1), // buffered to avoid blocking
			eventChs:      xsync.NewMap[chan *WakeEvent, struct{}](),
			cfg:           cfg,
			routeHelper: routeHelper{
				hc: monitor.NewMonitor(r),
			},
			dependsOn: make([]*dependency, 0, len(cfg.DependsOn)),
		}
	}

	var depErrors gperr.Builder
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

		var depRoute types.Route
		var ok bool

		// try to find the dependency in the same provider and the same docker compose project first
		if cont != nil {
			depRoute, ok = r.GetProvider().FindService(docker.DockerComposeProject(cont), dep)
		}

		if !ok {
			depRoute, ok = entrypoint.FromCtx(parent.Context()).GetRoute(dep)
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
			depCfg = new(types.IdlewatcherConfig)
			depCfg.IdlewatcherConfigBase = cfg.IdlewatcherConfigBase
			depCfg.IdleTimeout = neverTick // disable auto sleep for dependencies
		} else if depCfg.IdleTimeout > 0 && depCfg.IdleTimeout != neverTick {
			depErrors.Addf("dependency %q has positive idle timeout %s", dep, depCfg.IdleTimeout)
			continue
		}

		if depCfg.Docker == nil && depCfg.Proxmox == nil {
			depCont := depRoute.ContainerInfo()
			if depCont != nil {
				depCfg.Docker = &types.DockerConfig{
					DockerCfg:     depCont.DockerCfg,
					ContainerID:   depCont.ContainerID,
					ContainerName: depCont.ContainerName,
				}
				depCfg.DependsOn = docker.Dependencies(depCont)
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

	if pOld := w.provider.Load(); pOld != nil { // it's a reload, close the old provider
		pOld.Close()
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
		p, err = provider.NewDockerProvider(cfg.Docker.DockerCfg, cfg.Docker.ContainerID)
		kind = "docker"
	default:
		p, err = provider.NewProxmoxProvider(parent.Context(), cfg.Proxmox.Node, cfg.Proxmox.VMID)
		kind = "proxmox"
	}
	targetURL := r.TargetURL()
	if targetURL == nil {
		return nil, errors.New("target URL is not set")
	}
	w.l = log.With().
		Str("kind", kind).
		Str("container", cfg.ContainerName()).
		Str("url", targetURL.String()).
		Logger()

	if cfg.IdleTimeout != neverTick {
		w.l = w.l.With().Str("idle_timeout", strutils.FormatDuration(cfg.IdleTimeout)).Logger()
	}

	if err != nil {
		return nil, err
	}
	w.provider.Store(p)

	switch r := r.(type) {
	case types.ReverseProxyRoute:
		w.rp = r.ReverseProxy()
	case types.StreamRoute:
		w.stream = r.Stream()
	default:
		p.Close()
		return nil, w.newWatcherError(fmt.Errorf("unexpected route type: %T", r))
	}
	w.route = r

	ctx, cancel := context.WithTimeout(parent.Context(), reqTimeout)
	defer cancel()
	status, err := p.ContainerStatus(ctx)
	if err != nil {
		p.Close()
		return nil, w.newWatcherError(err)
	}
	w.state.Store(&containerState{status: status})

	// when more providers are added, we need to add a new case here.
	switch p := p.(type) { //nolint:gocritic
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

			watcherMapMu.Lock()
			delete(watcherMap, key)
			watcherMapMu.Unlock()

			if errors.Is(cause, errCauseReload) {
				// no log
			} else if errors.Is(cause, errCauseContainerDestroy) || errors.Is(cause, task.ErrProgramExiting) || errors.Is(cause, config.ErrConfigChanged) {
				w.l.Info().Msg("idlewatcher stopped")
			} else {
				w.l.Err(cause).Msg("idlewatcher stopped unexpectedly")
			}

			w.idleTicker.Stop()
			w.healthTicker.Stop()
			w.setReady()
			close(w.readyNotifyCh)
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
		w.sendEvent(WakeEventError, "Failed to wake dependencies", err)
		return w.newWatcherError(err)
	}

	if w.wakeInProgress() {
		w.l.Debug().Msg("already starting, ignoring duplicate start event")
		return nil
	}

	// wake itself.
	// use container name instead of Key() here as the container id will change on restart (docker).
	containerName := w.cfg.ContainerName()
	_, err, _ := singleFlight.Do(containerName, func() (any, error) {
		err := w.wakeIfStopped(ctx)
		if err != nil {
			w.sendEvent(WakeEventError, "Failed to start "+containerName, err)
		} else {
			w.sendEvent(WakeEventContainerWoke, containerName+" started successfully", nil)
			w.sendEvent(WakeEventWaitingReady, "Waiting for "+containerName+" to be ready...", nil)
		}
		return nil, err
	})
	if err != nil {
		return w.newWatcherError(err)
	}

	return nil
}

func (w *Watcher) wakeInProgress() bool {
	state := w.state.Load()
	if state == nil {
		return false
	}
	return !state.startedAt.IsZero()
}

func (w *Watcher) wakeDependencies(ctx context.Context) error {
	if len(w.dependsOn) == 0 {
		return nil
	}

	errs := errgroup.Group{}
	for _, dep := range w.dependsOn {
		if dep.wakeInProgress() {
			w.l.Debug().Str("dep", dep.cfg.ContainerName()).Msg("dependency already starting, ignoring duplicate start event")
			continue
		}
		errs.Go(func() error {
			w.sendEvent(WakeEventWakingDep, "Waking dependency: "+dep.cfg.ContainerName(), nil)
			if err := dep.Wake(ctx); err != nil {
				return err
			}
			w.sendEvent(WakeEventDepReady, "Dependency woke: "+dep.cfg.ContainerName(), nil)
			if dep.waitHealthy {
				// initial health check before starting the ticker
				if h, err := dep.hc.CheckHealth(); err != nil {
					return err
				} else if h.Healthy {
					return nil
				}

				tick := time.NewTicker(idleWakerCheckInterval)
				defer tick.Stop()
				for {
					select {
					case <-ctx.Done():
						return w.newDepError("wait_healthy", dep, context.Cause(ctx))
					case <-tick.C:
						if h, err := dep.hc.CheckHealth(); err != nil {
							return err
						} else if h.Healthy {
							return nil
						}
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
	p := w.provider.Load()
	if p == nil {
		return errors.New("provider not set")
	}
	switch state.status {
	case idlewatcher.ContainerStatusStopped:
		w.sendEvent(WakeEventStarting, w.cfg.ContainerName()+" is starting...", nil)
		return p.ContainerStart(ctx)
	case idlewatcher.ContainerStatusPaused:
		w.sendEvent(WakeEventStarting, w.cfg.ContainerName()+" is unpausing...", nil)
		return p.ContainerUnpause(ctx)
	default:
		return fmt.Errorf("unexpected container status: %s", state.status)
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
	p := w.provider.Load()
	if p == nil {
		return errors.New("provider not set")
	}
	switch cfg.StopMethod {
	case types.ContainerStopMethodPause:
		err = p.ContainerPause(ctx)
	case types.ContainerStopMethodStop:
		err = p.ContainerStop(ctx, cfg.StopSignal, int(math.Ceil(cfg.StopTimeout.Seconds())))
	case types.ContainerStopMethodKill:
		err = p.ContainerKill(ctx, cfg.StopSignal)
	default:
		err = w.newWatcherError(fmt.Errorf("unexpected stop method: %q", cfg.StopMethod))
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
	p := w.provider.Load()
	if p == nil {
		return errors.New("provider not set")
	}
	defer p.Close()
	eventCh, errCh := p.Watch(w.Task().Context())

	for {
		select {
		case <-w.task.Context().Done():
			return w.task.FinishCause()
		case err := <-errCh:
			w.l.Err(err).Msg("watcher error")
		case e := <-eventCh:
			w.l.Debug().Stringer("action", e.Action).Msg("state changed")
			switch e.Action {
			case events.ActionContainerDestroy:
				return errCauseContainerDestroy
			case events.ActionForceReload:
				continue
			}
			w.resetIdleTimer()
			switch {
			case e.Action.IsContainerStart(): // create / start / unpause
				w.setStarting()
				w.healthTicker.Reset(idleWakerCheckInterval) // start health checking
				w.l.Info().Msg("awaken")
			case e.Action.IsContainerStop(): // stop / kill / die
				w.setNapping(idlewatcher.ContainerStatusStopped)
				w.idleTicker.Stop()
				w.healthTicker.Stop() // stop health checking
			case e.Action.IsContainerPause(): // pause
				w.setNapping(idlewatcher.ContainerStatusPaused)
				w.idleTicker.Stop()
				w.healthTicker.Stop() // stop health checking
			default:
				w.l.Debug().Stringer("action", e.Action).Msg("unexpected container action")
			}
		case <-w.healthTicker.C:
			// Only check health if container is starting (not ready yet)
			if w.running() && !w.ready() {
				ready, err := w.checkUpdateState()
				if err != nil {
					// Health check failed with error, stop health checking
					w.healthTicker.Stop()
					continue
				}
				if ready {
					// Container is now ready, notify waiting handlers
					w.healthTicker.Stop()
					w.resetIdleTimer()
				}
				// If not ready yet, keep checking on next tick
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
		for depdep := range depdeps.Iter {
			deps.Del(depdep)
		}
	}
	newDepOn := make([]string, 0, deps.Len())
	newDeps := make([]*dependency, 0, deps.Len())
	for _, dep := range deps.Iter {
		newDepOn = append(newDepOn, dep.cfg.ContainerName())
		newDeps = append(newDeps, dep)
	}
	w.cfg.DependsOn = newDepOn
	w.dependsOn = newDeps
}

func (w *Watcher) dependencies() *ordered.Map[string, *dependency] {
	deps := ordered.NewMap[string, *dependency]()
	for _, dep := range w.dependsOn {
		deps.Set(dep.Key(), dep)
		for _, depdep := range dep.dependencies().Iter {
			deps.Set(depdep.Key(), depdep)
		}
	}
	return deps
}
