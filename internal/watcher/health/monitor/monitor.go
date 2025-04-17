package monitor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/notif"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/atomic"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	"github.com/yusing/go-proxy/internal/watcher/health"
)

type (
	HealthCheckFunc func() (result *health.HealthCheckResult, err error)
	monitor         struct {
		service string
		config  *health.HealthCheckConfig
		url     atomic.Value[*url.URL]

		status     atomic.Value[health.Status]
		lastResult atomic.Value[*health.HealthCheckResult]

		checkHealth HealthCheckFunc
		startTime   time.Time

		task *task.Task
	}
)

var ErrNegativeInterval = errors.New("negative interval")

func NewMonitor(r routes.Route) health.HealthMonCheck {
	var mon health.HealthMonCheck
	if r.IsAgent() {
		mon = NewAgentProxiedMonitor(r.Agent(), r.HealthCheckConfig(), AgentTargetFromURL(r.TargetURL()))
	} else {
		switch r := r.(type) {
		case routes.HTTPRoute:
			mon = NewHTTPHealthMonitor(r.TargetURL(), r.HealthCheckConfig())
		case routes.StreamRoute:
			mon = NewRawHealthMonitor(r.TargetURL(), r.HealthCheckConfig())
		default:
			logging.Panic().Msgf("unexpected route type: %T", r)
		}
	}
	if r.IsDocker() {
		cont := r.ContainerInfo()
		client, err := docker.NewClient(cont.DockerHost)
		if err != nil {
			return mon
		}
		r.Task().OnCancel("close_docker_client", client.Close)
		return NewDockerHealthMonitor(client, cont.ContainerID, r.Name(), r.HealthCheckConfig(), mon)
	}
	return mon
}

func newMonitor(url *url.URL, config *health.HealthCheckConfig, healthCheckFunc HealthCheckFunc) *monitor {
	mon := &monitor{
		config:      config,
		checkHealth: healthCheckFunc,
		startTime:   time.Now(),
	}
	mon.url.Store(url)
	mon.status.Store(health.StatusHealthy)
	return mon
}

func (mon *monitor) ContextWithTimeout(cause string) (ctx context.Context, cancel context.CancelFunc) {
	if mon.task != nil {
		return context.WithTimeoutCause(mon.task.Context(), mon.config.Timeout, errors.New(cause))
	}
	return context.WithTimeoutCause(context.Background(), mon.config.Timeout, errors.New(cause))
}

// Start implements task.TaskStarter.
func (mon *monitor) Start(parent task.Parent) gperr.Error {
	if mon.config.Interval <= 0 {
		return gperr.Wrap(ErrNegativeInterval)
	}

	mon.service = parent.Name()
	mon.task = parent.Subtask("health_monitor", true)

	go func() {
		logger := logging.With().Str("name", mon.service).Logger()

		defer func() {
			if mon.status.Load() != health.StatusError {
				mon.status.Store(health.StatusUnknown)
			}
			mon.task.Finish(nil)
		}()

		if err := mon.checkUpdateHealth(); err != nil {
			logger.Err(err).Msg("healthchecker failure")
			return
		}

		ticker := time.NewTicker(mon.config.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-mon.task.Context().Done():
				return
			case <-ticker.C:
				err := mon.checkUpdateHealth()
				if err != nil {
					logger.Err(err).Msg("healthchecker failure")
					return
				}
			}
		}
	}()
	return nil
}

// Task implements task.TaskStarter.
func (mon *monitor) Task() *task.Task {
	return mon.task
}

// Finish implements task.TaskFinisher.
func (mon *monitor) Finish(reason any) {
	mon.task.Finish(reason)
}

// UpdateURL implements HealthChecker.
func (mon *monitor) UpdateURL(url *url.URL) {
	mon.url.Store(url)
}

// URL implements HealthChecker.
func (mon *monitor) URL() *url.URL {
	return mon.url.Load()
}

// Config implements HealthChecker.
func (mon *monitor) Config() *health.HealthCheckConfig {
	return mon.config
}

// Status implements HealthMonitor.
func (mon *monitor) Status() health.Status {
	return mon.status.Load()
}

// Uptime implements HealthMonitor.
func (mon *monitor) Uptime() time.Duration {
	return time.Since(mon.startTime)
}

// Latency implements HealthMonitor.
func (mon *monitor) Latency() time.Duration {
	res := mon.lastResult.Load()
	if res == nil {
		return 0
	}
	return res.Latency
}

// Name implements HealthMonitor.
func (mon *monitor) Name() string {
	parts := strutils.SplitRune(mon.service, '/')
	return parts[len(parts)-1]
}

// String implements fmt.Stringer of HealthMonitor.
func (mon *monitor) String() string {
	return mon.Name()
}

// MarshalMap implements health.HealthMonitor.
func (mon *monitor) MarshalMap() map[string]any {
	res := mon.lastResult.Load()
	if res == nil {
		res = &health.HealthCheckResult{
			Healthy: true,
		}
	}

	return (&health.JSONRepresentation{
		Name:     mon.service,
		Config:   mon.config,
		Status:   mon.status.Load(),
		Started:  mon.startTime,
		Uptime:   mon.Uptime(),
		Latency:  res.Latency,
		LastSeen: GetLastSeen(mon.service),
		Detail:   res.Detail,
		URL:      mon.url.Load(),
	}).MarshalMap()
}

func (mon *monitor) checkUpdateHealth() error {
	logger := logging.With().Str("name", mon.Name()).Logger()
	result, err := mon.checkHealth()
	if err != nil {
		defer mon.task.Finish(err)
		mon.status.Store(health.StatusError)
		if !errors.Is(err, context.Canceled) {
			return fmt.Errorf("check health: %w", err)
		}
		return nil
	}

	mon.lastResult.Store(result)
	var status health.Status
	if result.Healthy {
		status = health.StatusHealthy
		UpdateLastSeen(mon.service)
	} else {
		status = health.StatusUnhealthy
	}
	if result.Healthy != (mon.status.Swap(status) == health.StatusHealthy) {
		extras := notif.LogFields{
			{Name: "Service Name", Value: mon.service},
			{Name: "Time", Value: strutils.FormatTime(time.Now())},
		}
		if !result.Healthy {
			extras.Add("Last Seen", strutils.FormatLastSeen(GetLastSeen(mon.service)))
		}
		if mon.url.Load() != nil {
			extras.Add("Service URL", mon.url.Load().String())
		}
		if result.Detail != "" {
			extras.Add("Detail", result.Detail)
		}
		if result.Healthy {
			logger.Info().Msg("service is up")
			extras.Add("Ping", fmt.Sprintf("%d ms", result.Latency.Milliseconds()))
			notif.Notify(&notif.LogMessage{
				Title:  "✅ Service is up ✅",
				Extras: extras,
				Color:  notif.ColorSuccess,
			})
		} else {
			logger.Warn().Msg("service went down")
			notif.Notify(&notif.LogMessage{
				Title:  "❌ Service went down ❌",
				Extras: extras,
				Color:  notif.ColorError,
			})
		}
	}

	return nil
}
