package idlewatcher

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/docker"
	idlewatcher "github.com/yusing/go-proxy/internal/docker/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/task"
	U "github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/atomic"
	"github.com/yusing/go-proxy/internal/watcher"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

type (
	Watcher struct {
		_ U.NoCopy

		zerolog.Logger

		*waker
		*containerMeta
		*idlewatcher.Config

		client *docker.SharedClient
		state  atomic.Value[*containerState]

		stopByMethod StopCallback // send a docker command w.r.t. `stop_method`
		ticker       *time.Ticker
		lastReset    time.Time
		task         *task.Task
	}

	StopCallback func() error
)

var (
	watcherMap   = make(map[string]*Watcher)
	watcherMapMu sync.RWMutex

	errShouldNotReachHere = errors.New("should not reach here")
)

const dockerReqTimeout = 3 * time.Second

func registerWatcher(parent task.Parent, route route.Route, waker *waker) (*Watcher, error) {
	cfg := route.IdlewatcherConfig()

	if cfg.IdleTimeout == 0 {
		panic(errShouldNotReachHere)
	}

	cont := route.ContainerInfo()
	key := cont.ContainerID

	watcherMapMu.Lock()
	defer watcherMapMu.Unlock()
	w, ok := watcherMap[key]
	if !ok {
		client, err := docker.NewClient(cont.DockerHost)
		if err != nil {
			return nil, err
		}

		w = &Watcher{
			Logger: logging.With().Str("name", cont.ContainerName).Logger(),
			client: client,
			task:   parent.Subtask("idlewatcher." + cont.ContainerName),
			ticker: time.NewTicker(cfg.IdleTimeout),
		}
	}

	// FIXME: possible race condition here
	w.waker = waker
	w.containerMeta = &containerMeta{
		ContainerID:   cont.ContainerID,
		ContainerName: cont.ContainerName,
	}
	w.Config = cfg
	w.ticker.Reset(cfg.IdleTimeout)

	if cont.Running {
		w.setStarting()
	} else {
		w.setNapping()
	}

	if !ok {
		w.stopByMethod = w.getStopCallback()
		watcherMap[key] = w

		go func() {
			cause := w.watchUntilDestroy()

			watcherMapMu.Lock()
			defer watcherMapMu.Unlock()
			delete(watcherMap, key)

			w.ticker.Stop()
			w.client.Close()
			w.task.Finish(cause)
		}()
	}

	return w, nil
}

func (w *Watcher) Wake() error {
	return w.wakeIfStopped()
}

// WakeDebug logs a debug message related to waking the container.
func (w *Watcher) WakeDebug() *zerolog.Event {
	//nolint:zerologlint
	return w.Debug().Str("action", "wake")
}

func (w *Watcher) WakeTrace() *zerolog.Event {
	//nolint:zerologlint
	return w.Trace().Str("action", "wake")
}

func (w *Watcher) WakeError(err error) {
	w.Err(err).Str("action", "wake").Msg("error")
}

func (w *Watcher) wakeIfStopped() error {
	if w.running() {
		return nil
	}

	status, err := w.containerStatus()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(w.task.Context(), w.WakeTimeout)
	defer cancel()

	// !Hard coded here since theres no constants from Docker API
	switch status {
	case "exited", "dead":
		return w.containerStart(ctx)
	case "paused":
		return w.containerUnpause(ctx)
	case "running":
		return nil
	default:
		return gperr.Errorf("unexpected container status: %s", status)
	}
}

func (w *Watcher) getStopCallback() StopCallback {
	var cb func(context.Context) error
	switch w.StopMethod {
	case idlewatcher.StopMethodPause:
		cb = w.containerPause
	case idlewatcher.StopMethodStop:
		cb = w.containerStop
	case idlewatcher.StopMethodKill:
		cb = w.containerKill
	default:
		panic(errShouldNotReachHere)
	}
	return func() error {
		ctx, cancel := context.WithTimeout(w.task.Context(), time.Duration(w.StopTimeout)*time.Second)
		defer cancel()
		return cb(ctx)
	}
}

func (w *Watcher) resetIdleTimer() {
	w.Trace().Msg("reset idle timer")
	w.ticker.Reset(w.IdleTimeout)
	w.lastReset = time.Now()
}

func (w *Watcher) expires() time.Time {
	return w.lastReset.Add(w.IdleTimeout)
}

func (w *Watcher) getEventCh(ctx context.Context, dockerWatcher *watcher.DockerWatcher) (eventCh <-chan events.Event, errCh <-chan gperr.Error) {
	eventCh, errCh = dockerWatcher.EventsWithOptions(ctx, watcher.DockerListOptions{
		Filters: watcher.NewDockerFilter(
			watcher.DockerFilterContainer,
			watcher.DockerFilterContainerNameID(w.ContainerID),
			watcher.DockerFilterStart,
			watcher.DockerFilterStop,
			watcher.DockerFilterDie,
			watcher.DockerFilterKill,
			watcher.DockerFilterDestroy,
			watcher.DockerFilterPause,
			watcher.DockerFilterUnpause,
		),
	})
	return
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
	eventCtx, eventCancel := context.WithCancel(w.task.Context())
	defer eventCancel()

	dockerWatcher := watcher.NewDockerWatcher(w.client.DaemonHost())
	dockerEventCh, dockerEventErrCh := w.getEventCh(eventCtx, dockerWatcher)

	for {
		select {
		case <-w.task.Context().Done():
			return w.task.FinishCause()
		case err := <-dockerEventErrCh:
			if !err.Is(context.Canceled) {
				gperr.LogError("idlewatcher error", err, &w.Logger)
			}
			return err
		case e := <-dockerEventCh:
			switch {
			case e.Action == events.ActionContainerDestroy:
				w.setError(errors.New("container destroyed"))
				w.Info().Str("reason", "container destroyed").Msg("watcher stopped")
				return errors.New("container destroyed")
			// create / start / unpause
			case e.Action.IsContainerWake():
				w.setStarting()
				w.resetIdleTimer()
				w.Info().Msg("awaken")
			case e.Action.IsContainerSleep(): // stop / pause / kil
				w.setNapping()
				w.resetIdleTimer()
				w.ticker.Stop()
			default:
				w.Error().Msg("unexpected docker event: " + e.String())
			}
			// container name changed should also change the container id
			if w.ContainerName != e.ActorName {
				w.Debug().Msgf("renamed %s -> %s", w.ContainerName, e.ActorName)
				w.ContainerName = e.ActorName
			}
			if w.ContainerID != e.ActorID {
				w.Debug().Msgf("id changed %s -> %s", w.ContainerID, e.ActorID)
				w.ContainerID = e.ActorID
				// recreate event stream
				eventCancel()

				eventCtx, eventCancel = context.WithCancel(w.task.Context())
				defer eventCancel()
				dockerEventCh, dockerEventErrCh = w.getEventCh(eventCtx, dockerWatcher)
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
					w.Err(err).Msgf("container stop with method %q failed", w.StopMethod)
				default:
					w.Info().Str("reason", "idle timeout").Msg("container stopped")
				}
			}
		}
	}
}
