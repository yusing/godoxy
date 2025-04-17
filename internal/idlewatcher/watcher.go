package idlewatcher

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/idlewatcher/provider"
	idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
	net "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/task"
	U "github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/atomic"
	"github.com/yusing/go-proxy/internal/watcher/events"
	"github.com/yusing/go-proxy/internal/watcher/health"
	"github.com/yusing/go-proxy/internal/watcher/health/monitor"
)

type (
	routeHelper struct {
		rp     *reverseproxy.ReverseProxy
		stream net.Stream
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

		ticker *time.Ticker
		task   *task.Task
	}

	StopCallback func() error
)

const ContextKey = "idlewatcher.watcher"

var (
	watcherMap   = make(map[string]*Watcher)
	watcherMapMu sync.RWMutex
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
	causeReload           = gperr.New("reloaded")
	causeContainerDestroy = gperr.New("container destroyed")
)

const reqTimeout = 3 * time.Second

// TODO: fix stream type
func NewWatcher(parent task.Parent, r routes.Route) (*Watcher, error) {
	cfg := r.IdlewatcherConfig()
	key := cfg.Key()

	watcherMapMu.RLock()
	// if the watcher already exists, finish it
	w, exists := watcherMap[key]
	if exists {
		if w.cfg == cfg {
			// same address, likely two routes from the same container
			return w, nil
		}
		w.task.Finish(causeReload)
	}
	watcherMapMu.RUnlock()

	w = &Watcher{
		ticker: time.NewTicker(cfg.IdleTimeout),
		cfg:    cfg,
		routeHelper: routeHelper{
			hc: monitor.NewMonitor(r),
		},
	}

	var p idlewatcher.Provider
	var providerType string
	var err error
	switch {
	case cfg.Docker != nil:
		p, err = provider.NewDockerProvider(cfg.Docker.DockerHost, cfg.Docker.ContainerID)
		providerType = "docker"
	default:
		p, err = provider.NewProxmoxProvider(cfg.Proxmox.Node, cfg.Proxmox.VMID)
		providerType = "proxmox"
	}

	if err != nil {
		return nil, err
	}
	w.provider = p
	w.l = logging.With().
		Str("provider", providerType).
		Str("container", cfg.ContainerName()).
		Logger()

	switch r := r.(type) {
	case routes.ReverseProxyRoute:
		w.rp = r.ReverseProxy()
	case routes.StreamRoute:
		w.stream = r
	default:
		return nil, gperr.New("unexpected route type")
	}

	ctx, cancel := context.WithTimeout(parent.Context(), reqTimeout)
	defer cancel()
	status, err := w.provider.ContainerStatus(ctx)
	if err != nil {
		w.provider.Close()
		return nil, gperr.Wrap(err, "failed to get container status")
	}

	switch p := w.provider.(type) {
	case *provider.ProxmoxProvider:
		shutdownTimeout := max(time.Second, cfg.StopTimeout-idleWakerCheckTimeout)
		err = p.LXCSetShutdownTimeout(ctx, cfg.Proxmox.VMID, shutdownTimeout)
		if err != nil {
			w.l.Warn().Err(err).Msg("failed to set shutdown timeout")
		}
	}

	w.state.Store(&containerState{status: status})

	w.task = parent.Subtask("idlewatcher."+r.Name(), true)

	watcherMapMu.Lock()
	defer watcherMapMu.Unlock()
	watcherMap[key] = w
	go func() {
		cause := w.watchUntilDestroy()
		if cause.Is(causeContainerDestroy) || cause.Is(task.ErrProgramExiting) {
			watcherMapMu.Lock()
			defer watcherMapMu.Unlock()
			delete(watcherMap, key)
			w.l.Info().Msg("idlewatcher stopped")
		} else if !cause.Is(causeReload) {
			gperr.LogError("idlewatcher stopped unexpectedly", cause, &w.l)
		}

		w.ticker.Stop()
		w.provider.Close()
		w.task.Finish(cause)
	}()
	if exists {
		w.l.Info().Msg("idlewatcher reloaded")
	} else {
		w.l.Info().Msg("idlewatcher started")
	}
	return w, nil
}

func (w *Watcher) Key() string {
	return w.cfg.Key()
}

func (w *Watcher) Wake() error {
	return w.wakeIfStopped()
}

func (w *Watcher) wakeIfStopped() error {
	state := w.state.Load()
	if state.status == idlewatcher.ContainerStatusRunning {
		w.l.Debug().Msg("container is already running")
		return nil
	}

	ctx, cancel := context.WithTimeout(w.task.Context(), w.cfg.WakeTimeout)
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

func (w *Watcher) stopByMethod() error {
	if !w.running() {
		return nil
	}

	cfg := w.cfg
	ctx, cancel := context.WithTimeout(w.task.Context(), cfg.StopTimeout)
	defer cancel()

	switch cfg.StopMethod {
	case idlewatcher.StopMethodPause:
		return w.provider.ContainerPause(ctx)
	case idlewatcher.StopMethodStop:
		return w.provider.ContainerStop(ctx, cfg.StopSignal, int(cfg.StopTimeout.Seconds()))
	case idlewatcher.StopMethodKill:
		return w.provider.ContainerKill(ctx, cfg.StopSignal)
	default:
		return gperr.Errorf("unexpected stop method: %q", cfg.StopMethod)
	}
}

func (w *Watcher) resetIdleTimer() {
	w.ticker.Reset(w.cfg.IdleTimeout)
	w.lastReset.Store(time.Now())
}

func (w *Watcher) expires() time.Time {
	if !w.running() {
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
func (w *Watcher) watchUntilDestroy() (returnCause gperr.Error) {
	eventCh, errCh := w.provider.Watch(w.Task().Context())

	for {
		select {
		case <-w.task.Context().Done():
			return gperr.Wrap(w.task.FinishCause())
		case err := <-errCh:
			return err
		case e := <-eventCh:
			w.l.Debug().Stringer("action", e.Action).Msg("state changed")
			if e.Action == events.ActionContainerDestroy {
				return causeContainerDestroy
			}
			w.resetIdleTimer()
			switch {
			case e.Action.IsContainerStart(): // create / start / unpause
				w.setStarting()
				w.l.Info().Msg("awaken")
			case e.Action.IsContainerStop(): // stop / kill / die
				w.setNapping(idlewatcher.ContainerStatusStopped)
				w.ticker.Stop()
			case e.Action.IsContainerPause(): // pause
				w.setNapping(idlewatcher.ContainerStatusPaused)
				w.ticker.Stop()
			default:
				w.l.Error().Stringer("action", e.Action).Msg("unexpected container action")
			}
		case <-w.ticker.C:
			w.ticker.Stop()
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
					w.l.Info().Str("reason", "idle timeout").Msg("container stopped")
				}
			}
		}
	}
}

func fmtErr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
