package idlewatcher

import (
	"time"

	"github.com/yusing/go-proxy/internal/gperr"
	idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/types"
)

// Start implements health.HealthMonitor.
func (w *Watcher) Start(parent task.Parent) gperr.Error {
	w.task.OnCancel("route_cleanup", func() {
		parent.Finish(w.task.FinishCause())
	})
	return nil
}

// Task implements health.HealthMonitor.
func (w *Watcher) Task() *task.Task {
	return w.task
}

// Finish implements health.HealthMonitor.
func (w *Watcher) Finish(reason any) {
	if w.stream != nil {
		w.stream.Close()
	}
}

// Name implements health.HealthMonitor.
func (w *Watcher) Name() string {
	return w.cfg.ContainerName()
}

// String implements health.HealthMonitor.
func (w *Watcher) String() string {
	return w.Name()
}

// Uptime implements health.HealthMonitor.
func (w *Watcher) Uptime() time.Duration {
	return 0
}

// Latency implements health.HealthMonitor.
func (w *Watcher) Latency() time.Duration {
	return 0
}

// Status implements health.HealthMonitor.
func (w *Watcher) Status() types.HealthStatus {
	state := w.state.Load()
	if state.err != nil {
		return types.StatusError
	}
	if state.ready {
		return types.StatusHealthy
	}
	if state.status == idlewatcher.ContainerStatusRunning {
		return types.StatusStarting
	}
	return types.StatusNapping
}

// Detail implements health.HealthMonitor.
func (w *Watcher) Detail() string {
	state := w.state.Load()
	if state.err != nil {
		return state.err.Error()
	}
	if !state.ready {
		return "not ready"
	}
	if state.status == idlewatcher.ContainerStatusRunning {
		return "starting"
	}
	return "napping"
}

// MarshalJSON implements health.HealthMonitor.
func (w *Watcher) MarshalJSON() ([]byte, error) {
	url := w.hc.URL()
	if url.Port() == "0" {
		url = nil
	}
	var detail string
	if err := w.error(); err != nil {
		detail = err.Error()
	}
	return (&types.HealthJSONRepr{
		Name:   w.Name(),
		Status: w.Status(),
		Config: &types.HealthCheckConfig{
			Interval: idleWakerCheckInterval,
			Timeout:  idleWakerCheckTimeout,
		},
		URL:    url,
		Detail: detail,
	}).MarshalJSON()
}

func (w *Watcher) checkUpdateState() (ready bool, err error) {
	// the new container info not yet updated
	if w.hc.URL().Host == "" {
		return false, nil
	}

	state := w.state.Load()

	// Check if container has been starting for too long (timeout after WakeTimeout)
	if !state.startedAt.IsZero() {
		elapsed := time.Since(state.startedAt)
		if elapsed > w.cfg.WakeTimeout {
			err := gperr.Errorf("container failed to become ready within %v (started at %v, %d health check attempts)",
				w.cfg.WakeTimeout, state.startedAt, state.healthTries)
			w.l.Error().
				Dur("elapsed", elapsed).
				Time("started_at", state.startedAt).
				Int("health_tries", state.healthTries).
				Msg("container startup timeout")
			w.setError(err)
			return false, err
		}
	}

	res, err := w.hc.CheckHealth()
	if err != nil {
		w.l.Debug().Err(err).Msg("health check error")
		w.setError(err)
		return false, err
	}

	if res.Healthy {
		w.l.Debug().
			Dur("startup_time", time.Since(state.startedAt)).
			Int("health_tries", state.healthTries+1).
			Msg("container ready")
		w.setReady()
		return true, nil
	}

	// Health check failed, increment counter and log
	newHealthTries := state.healthTries + 1
	w.state.Store(&containerState{
		status:      state.status,
		ready:       false,
		err:         state.err,
		startedAt:   state.startedAt,
		healthTries: newHealthTries,
	})

	w.l.Debug().
		Int("health_tries", newHealthTries).
		Dur("elapsed", time.Since(state.startedAt)).
		Msg("health check failed, still starting")

	return false, nil
}
