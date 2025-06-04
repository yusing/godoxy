package idlewatcher

import (
	"time"

	"github.com/yusing/go-proxy/internal/gperr"
	idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/watcher/health"
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
func (w *Watcher) Status() health.Status {
	state := w.state.Load()
	if state.err != nil {
		return health.StatusError
	}
	if state.ready {
		return health.StatusHealthy
	}
	if state.status == idlewatcher.ContainerStatusRunning {
		return health.StatusStarting
	}
	return health.StatusNapping
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
	return (&health.JSONRepresentation{
		Name:   w.Name(),
		Status: w.Status(),
		Config: dummyHealthCheckConfig,
		URL:    url,
		Detail: detail,
	}).MarshalJSON()
}

func (w *Watcher) checkUpdateState() (ready bool, err error) {
	// already ready
	if w.ready() {
		return true, nil
	}

	if !w.running() {
		return false, nil
	}

	// the new container info not yet updated
	if w.hc.URL().Host == "" {
		return false, nil
	}

	res, err := w.hc.CheckHealth()
	if err != nil {
		w.setError(err)
		return false, err
	}

	if res.Healthy {
		w.setReady()
		return true, nil
	}
	w.setStarting()
	return false, nil
}
