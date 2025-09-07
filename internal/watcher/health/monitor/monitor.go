package monitor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/notif"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/types"
	"github.com/yusing/go-proxy/internal/utils/atomic"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type (
	HealthCheckFunc func() (result *types.HealthCheckResult, err error)
	monitor         struct {
		service string
		config  *types.HealthCheckConfig
		url     atomic.Value[*url.URL]

		status     atomic.Value[types.HealthStatus]
		lastResult atomic.Value[*types.HealthCheckResult]

		checkHealth HealthCheckFunc
		startTime   time.Time

		isZeroPort bool

		notifyFunc           notif.NotifyFunc
		numConsecFailures    atomic.Int64
		downNotificationSent atomic.Bool

		task *task.Task
	}
)

var ErrNegativeInterval = gperr.New("negative interval")

func NewMonitor(r types.Route) types.HealthMonCheck {
	var mon types.HealthMonCheck
	if r.IsAgent() {
		mon = NewAgentProxiedMonitor(r.GetAgent(), r.HealthCheckConfig(), AgentTargetFromURL(&r.TargetURL().URL))
	} else {
		switch r := r.(type) {
		case types.HTTPRoute:
			mon = NewHTTPHealthMonitor(&r.TargetURL().URL, r.HealthCheckConfig())
		case types.StreamRoute:
			mon = NewRawHealthMonitor(&r.TargetURL().URL, r.HealthCheckConfig())
		default:
			log.Panic().Msgf("unexpected route type: %T", r)
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

func newMonitor(u *url.URL, config *types.HealthCheckConfig, healthCheckFunc HealthCheckFunc) *monitor {
	if config.Retries == 0 {
		if config.Interval == 0 {
			config.Interval = common.HealthCheckIntervalDefault
		}
		config.Retries = int64(common.HealthCheckDownNotifyDelayDefault / config.Interval)
	}
	mon := &monitor{
		config:      config,
		checkHealth: healthCheckFunc,
		startTime:   time.Now(),
		notifyFunc:  notif.Notify,
	}
	if u == nil {
		u = &url.URL{}
	}
	mon.url.Store(u)
	mon.status.Store(types.StatusHealthy)

	port := u.Port()
	mon.isZeroPort = port == "" || port == "0"
	if mon.isZeroPort {
		mon.status.Store(types.StatusUnknown)
		mon.lastResult.Store(&types.HealthCheckResult{Healthy: false, Detail: "no port detected"})
	}
	return mon
}

func (mon *monitor) ContextWithTimeout(cause string) (ctx context.Context, cancel context.CancelFunc) {
	switch {
	case mon.config.BaseContext != nil:
		ctx = mon.config.BaseContext()
	case mon.task != nil:
		ctx = mon.task.Context()
	default:
		ctx = context.Background()
	}
	return context.WithTimeoutCause(ctx, mon.config.Timeout, errors.New(cause))
}

// Start implements task.TaskStarter.
func (mon *monitor) Start(parent task.Parent) gperr.Error {
	if mon.config.Interval <= 0 {
		return ErrNegativeInterval
	}

	if mon.isZeroPort {
		return nil
	}

	mon.service = parent.Name()
	mon.task = parent.Subtask("health_monitor", true)

	go func() {
		logger := log.With().Str("name", mon.service).Logger()

		defer func() {
			if mon.status.Load() != types.StatusError {
				mon.status.Store(types.StatusUnhealthy)
			}
			mon.task.Finish(nil)
		}()

		failures := 0

		if err := mon.checkUpdateHealth(); err != nil {
			logger.Err(err).Msg("healthchecker error")
			failures++
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
					logger.Err(err).Msg("healthchecker error")
					failures++
				} else {
					failures = 0
				}
				if failures >= 5 {
					mon.status.Store(types.StatusError)
					mon.task.Finish(err)
					logger.Error().Msg("healthchecker stopped after 5 trials")
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
func (mon *monitor) Config() *types.HealthCheckConfig {
	return mon.config
}

// Status implements HealthMonitor.
func (mon *monitor) Status() types.HealthStatus {
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

// Detail implements HealthMonitor.
func (mon *monitor) Detail() string {
	res := mon.lastResult.Load()
	if res == nil {
		return ""
	}
	return res.Detail
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

var resHealthy = types.HealthCheckResult{Healthy: true}

// MarshalJSON implements health.HealthMonitor.
func (mon *monitor) MarshalJSON() ([]byte, error) {
	res := mon.lastResult.Load()
	if res == nil {
		res = &resHealthy
	}

	return (&types.HealthJSONRepr{
		Name:     mon.service,
		Config:   mon.config,
		Status:   mon.status.Load(),
		Started:  mon.startTime,
		Uptime:   mon.Uptime(),
		Latency:  res.Latency,
		LastSeen: GetLastSeen(mon.service),
		Detail:   res.Detail,
		URL:      mon.url.Load(),
	}).MarshalJSON()
}

func (mon *monitor) checkUpdateHealth() error {
	logger := log.With().Str("name", mon.Name()).Logger()
	result, err := mon.checkHealth()

	var lastStatus types.HealthStatus
	switch {
	case err != nil:
		result = &types.HealthCheckResult{Healthy: false, Detail: err.Error()}
		lastStatus = mon.status.Swap(types.StatusError)
	case result.Healthy:
		lastStatus = mon.status.Swap(types.StatusHealthy)
		UpdateLastSeen(mon.service)
	default:
		lastStatus = mon.status.Swap(types.StatusUnhealthy)
	}
	mon.lastResult.Store(result)

	// change of status
	if result.Healthy != (lastStatus == types.StatusHealthy) {
		if result.Healthy {
			mon.notifyServiceUp(&logger, result)
			mon.numConsecFailures.Store(0)
			mon.downNotificationSent.Store(false) // Reset notification state when service comes back up
		} else if mon.config.Retries < 0 {
			// immediate notification when retries < 0
			mon.notifyServiceDown(&logger, result)
			mon.downNotificationSent.Store(true)
		}
	}

	// if threshold >= 0, notify after threshold consecutive failures (but only once)
	if !result.Healthy && mon.config.Retries >= 0 {
		failureCount := mon.numConsecFailures.Add(1)
		if failureCount >= mon.config.Retries && !mon.downNotificationSent.Load() {
			mon.notifyServiceDown(&logger, result)
			mon.downNotificationSent.Store(true)
		}
	}

	return err
}

func (mon *monitor) notifyServiceUp(logger *zerolog.Logger, result *types.HealthCheckResult) {
	logger.Info().Msg("service is up")
	extras := mon.buildNotificationExtras(result)
	extras.Add("Ping", fmt.Sprintf("%d ms", result.Latency.Milliseconds()))
	mon.notifyFunc(&notif.LogMessage{
		Level: zerolog.InfoLevel,
		Title: "✅ Service is up ✅",
		Body:  extras,
		Color: notif.ColorSuccess,
	})
}

func (mon *monitor) notifyServiceDown(logger *zerolog.Logger, result *types.HealthCheckResult) {
	logger.Warn().Msg("service went down")
	extras := mon.buildNotificationExtras(result)
	extras.Add("Last Seen", strutils.FormatLastSeen(GetLastSeen(mon.service)))
	mon.notifyFunc(&notif.LogMessage{
		Level: zerolog.WarnLevel,
		Title: "❌ Service went down ❌",
		Body:  extras,
		Color: notif.ColorError,
	})
}

func (mon *monitor) buildNotificationExtras(result *types.HealthCheckResult) notif.FieldsBody {
	extras := notif.FieldsBody{
		{Name: "Service Name", Value: mon.service},
		{Name: "Time", Value: strutils.FormatTime(time.Now())},
	}
	if mon.url.Load() != nil {
		extras.Add("Service URL", mon.url.Load().String())
	}
	if result.Detail != "" {
		extras.Add("Detail", result.Detail)
	}
	return extras
}
