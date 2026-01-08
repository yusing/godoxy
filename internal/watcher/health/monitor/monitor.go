package monitor

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/synk"
	"github.com/yusing/goutils/task"
)

type (
	HealthCheckFunc func(url *url.URL) (result types.HealthCheckResult, err error)
	monitor         struct {
		service string
		config  types.HealthCheckConfig
		url     synk.Value[*url.URL]

		status     synk.Value[types.HealthStatus]
		lastResult synk.Value[types.HealthCheckResult]

		checkHealth HealthCheckFunc
		startTime   time.Time

		notifyFunc           notif.NotifyFunc
		numConsecFailures    atomic.Int64
		downNotificationSent atomic.Bool

		task *task.Task
	}
)

var ErrNegativeInterval = gperr.New("negative interval")

func NewMonitor(r types.Route) types.HealthMonCheck {
	target := &r.TargetURL().URL

	var mon types.HealthMonCheck
	if r.IsAgent() {
		mon = NewAgentProxiedMonitor(r.HealthCheckConfig(), r.GetAgent(), target)
	} else {
		switch r := r.(type) {
		case types.ReverseProxyRoute:
			mon = NewHTTPHealthMonitor(r.HealthCheckConfig(), target)
		case types.FileServerRoute:
			mon = NewFileServerHealthMonitor(r.HealthCheckConfig(), r.RootPath())
		case types.StreamRoute:
			mon = NewStreamHealthMonitor(r.HealthCheckConfig(), target)
		default:
			log.Panic().Msgf("unexpected route type: %T", r)
		}
	}
	if r.IsDocker() {
		cont := r.ContainerInfo()
		client, err := docker.NewClient(cont.DockerCfg, true)
		if err != nil {
			return mon
		}
		r.Task().OnCancel("close_docker_client", client.Close)

		fallback := mon
		return NewDockerHealthMonitor(r.HealthCheckConfig(), client, cont.ContainerID, fallback)
	}
	return mon
}

func (mon *monitor) init(u *url.URL, cfg types.HealthCheckConfig, healthCheckFunc HealthCheckFunc) *monitor {
	if state := config.WorkingState.Load(); state != nil {
		cfg.ApplyDefaults(state.Value().Defaults.HealthCheck)
	} else {
		cfg.ApplyDefaults(types.HealthCheckConfig{}) // use defaults from constants
	}
	mon.config = cfg
	mon.checkHealth = healthCheckFunc
	mon.startTime = time.Now()
	mon.notifyFunc = notif.Notify
	mon.status.Store(types.StatusHealthy)
	mon.lastResult.Store(types.HealthCheckResult{Healthy: true, Detail: "started"})

	if u == nil {
		mon.url.Store(&url.URL{})
	} else {
		mon.url.Store(u)
	}
	return nil
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
	return context.WithTimeoutCause(ctx, mon.config.Timeout, gperr.New(cause))
}

func (mon *monitor) CheckHealth() (types.HealthCheckResult, error) {
	return mon.checkHealth(mon.url.Load())
}

// Start implements task.TaskStarter.
func (mon *monitor) Start(parent task.Parent) gperr.Error {
	if mon.config.Interval <= 0 {
		return ErrNegativeInterval
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

		// add a random delay between 0 and 10 seconds to avoid thundering herd
		time.Sleep(time.Duration(rand.Intn(10)) * time.Second)

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
	if mon.task != nil {
		mon.task.Finish(reason)
	}
}

// UpdateURL implements HealthChecker.
func (mon *monitor) UpdateURL(url *url.URL) {
	if url == nil {
		log.Warn().Msg("attempting to update health monitor URL with nil")
		return
	}
	mon.url.Store(url)
}

// URL implements HealthChecker.
func (mon *monitor) URL() *url.URL {
	return mon.url.Load()
}

// Config implements HealthChecker.
func (mon *monitor) Config() *types.HealthCheckConfig {
	return &mon.config
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
	return res.Latency
}

// Detail implements HealthMonitor.
func (mon *monitor) Detail() string {
	res := mon.lastResult.Load()
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

// MarshalJSON implements health.HealthMonitor.
func (mon *monitor) MarshalJSON() ([]byte, error) {
	res := mon.lastResult.Load()
	return (&types.HealthJSONRepr{
		Name:     mon.service,
		Config:   &mon.config,
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
	result, err := mon.checkHealth(mon.url.Load())

	var lastStatus types.HealthStatus
	switch {
	case err != nil:
		result = types.HealthCheckResult{Healthy: false, Detail: err.Error()}
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
			mon.notifyServiceUp(&logger, &result)
			mon.numConsecFailures.Store(0)
			mon.downNotificationSent.Store(false) // Reset notification state when service comes back up
		} else if mon.config.Retries < 0 {
			// immediate notification when retries < 0
			mon.notifyServiceDown(&logger, &result)
			mon.downNotificationSent.Store(true)
		}
	}

	// if threshold >= 0, notify after threshold consecutive failures (but only once)
	if !result.Healthy && mon.config.Retries >= 0 {
		failureCount := mon.numConsecFailures.Add(1)
		if failureCount >= mon.config.Retries && !mon.downNotificationSent.Load() {
			mon.notifyServiceDown(&logger, &result)
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
	logger.Warn().Str("detail", result.Detail).Msg("service went down")
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
