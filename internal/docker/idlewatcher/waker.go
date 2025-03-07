package idlewatcher

import (
	"time"

	"github.com/yusing/go-proxy/internal/docker/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/metrics"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
	net "github.com/yusing/go-proxy/internal/net/types"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/task"
	U "github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/watcher/health"
	"github.com/yusing/go-proxy/internal/watcher/health/monitor"
)

type (
	Waker = types.Waker
	waker struct {
		_ U.NoCopy

		rp     *reverseproxy.ReverseProxy
		stream net.Stream
		hc     health.HealthChecker
		metric *metrics.Gauge
	}
)

const (
	idleWakerCheckInterval = 100 * time.Millisecond
	idleWakerCheckTimeout  = time.Second
)

// TODO: support stream

func newWaker(parent task.Parent, route route.Route, rp *reverseproxy.ReverseProxy, stream net.Stream) (Waker, gperr.Error) {
	hcCfg := route.HealthCheckConfig()
	hcCfg.Timeout = idleWakerCheckTimeout

	waker := &waker{
		rp:     rp,
		stream: stream,
	}
	watcher, err := registerWatcher(parent, route, waker)
	if err != nil {
		return nil, gperr.Errorf("register watcher: %w", err)
	}

	switch {
	case route.IsAgent():
		waker.hc = monitor.NewAgentProxiedMonitor(route.Agent(), hcCfg, monitor.AgentTargetFromURL(route.TargetURL()))
	case rp != nil:
		waker.hc = monitor.NewHTTPHealthChecker(route.TargetURL(), hcCfg)
	case stream != nil:
		waker.hc = monitor.NewRawHealthChecker(route.TargetURL(), hcCfg)
	default:
		panic("both nil")
	}

	return watcher, nil
}

// lifetime should follow route provider.
func NewHTTPWaker(parent task.Parent, route route.Route, rp *reverseproxy.ReverseProxy) (Waker, gperr.Error) {
	return newWaker(parent, route, rp, nil)
}

func NewStreamWaker(parent task.Parent, route route.Route, stream net.Stream) (Waker, gperr.Error) {
	return newWaker(parent, route, nil, stream)
}

// Start implements health.HealthMonitor.
func (w *Watcher) Start(parent task.Parent) gperr.Error {
	w.task.OnCancel("route_cleanup", func() {
		parent.Finish(w.task.FinishCause())
		if w.metric != nil {
			w.metric.Reset()
		}
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
	return w.String()
}

// String implements health.HealthMonitor.
func (w *Watcher) String() string {
	return w.ContainerName
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
	if state.running {
		return health.StatusStarting
	}
	return health.StatusNapping
}

func (w *Watcher) checkUpdateState() (ready bool, err error) {
	// already ready
	if w.ready() {
		return true, nil
	}

	if w.metric != nil {
		defer w.metric.Set(float64(w.Status()))
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

// MarshalJSON implements health.HealthMonitor.
func (w *Watcher) MarshalJSON() ([]byte, error) {
	var url *net.URL
	if w.hc.URL().Port() != "0" {
		url = w.hc.URL()
	}
	var detail string
	if err := w.error(); err != nil {
		detail = err.Error()
	}
	return (&monitor.JSONRepresentation{
		Name:   w.Name(),
		Status: w.Status(),
		Config: w.hc.Config(),
		URL:    url,
		Detail: detail,
	}).MarshalJSON()
}
