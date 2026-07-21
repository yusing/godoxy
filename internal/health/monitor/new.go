package monitor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/health"
	healthcheck "github.com/yusing/godoxy/internal/health/check"
	"github.com/yusing/godoxy/internal/routing"
)

type (
	Result  = health.HealthCheckResult
	Monitor = health.HealthMonCheck
)

// NewMonitor creates a health monitor based on the route type and configuration.
//
// See internal/health/monitor/README.md for detailed health check flow and conditions.
func NewMonitor(r routing.Route) Monitor {
	target := &r.TargetURL().URL
	config := r.HealthCheckConfig()
	container := r.ContainerInfo()
	dockerHealthEnabled := container != nil && container.HealthCheckEnabled

	var mon Monitor
	if !dockerHealthEnabled || !config.Disable {
		if r.IsAgent() {
			mon = NewAgentProxiedMonitor(config, r.GetAgent(), target)
		} else {
			switch r := r.(type) {
			case routing.ReverseProxyRoute:
				mon = NewHTTPHealthMonitor(config, target)
			case routing.FileServerRoute:
				mon = NewFileServerHealthMonitor(config, r.RootPath())
			case routing.StreamRoute:
				mon = NewStreamHealthMonitor(config, target)
			default:
				log.Panic().Msgf("unexpected route type: %T", r)
			}
		}
	}
	if dockerHealthEnabled {
		displayURL := &url.URL{
			Scheme: "docker",
			Host:   container.DockerCfg.URL,
			Path:   "/containers/" + container.ContainerID + "/json",
		}
		return newDockerHealthMonitor(
			config,
			displayURL,
			container.ContainerID,
			mon,
			func() (*docker.SharedClient, error) {
				client, err := docker.NewClient(container.DockerCfg, true)
				if err != nil {
					return nil, err
				}
				r.Task().OnCancel("close_docker_client", client.Close)
				return client, nil
			},
		)
	}
	return mon
}

func NewHTTPHealthMonitor(config health.HealthCheckConfig, u *url.URL) Monitor {
	var method string
	if config.UseGet {
		method = http.MethodGet
	} else {
		method = http.MethodHead
	}

	var mon monitor
	mon.init(u, config, func(u *url.URL) (result Result, err error) {
		if u.Scheme == "h2c" {
			return healthcheck.H2C(mon.Context(), u, method, config.Path, config.Timeout)
		}
		return healthcheck.HTTP(u, method, config.Path, config.Timeout)
	})
	return &mon
}

func NewFileServerHealthMonitor(config health.HealthCheckConfig, path string) Monitor {
	var mon monitor
	mon.init(&url.URL{Scheme: "file", Host: path}, config, func(u *url.URL) (result Result, err error) {
		return healthcheck.FileServer(path)
	})
	return &mon
}

func NewStreamHealthMonitor(config health.HealthCheckConfig, targetURL *url.URL) Monitor {
	var mon monitor
	mon.init(targetURL, config, func(u *url.URL) (result Result, err error) {
		return healthcheck.Stream(mon.Context(), u, config.Timeout)
	})
	return &mon
}

func NewDockerHealthMonitor(config health.HealthCheckConfig, client *docker.SharedClient, containerID string, fallback Monitor) Monitor {
	displayURL := &url.URL{ // only for display purposes, no actual request is made
		Scheme: "docker",
		Host:   client.DaemonHost(),
		Path:   "/containers/" + containerID + "/json",
	}
	return newDockerHealthMonitor(
		config,
		displayURL,
		containerID,
		fallback,
		func() (*docker.SharedClient, error) { return client, nil },
	)
}

func newDockerHealthMonitor(
	config health.HealthCheckConfig,
	displayURL *url.URL,
	containerID string,
	fallback Monitor,
	newClient func() (*docker.SharedClient, error),
) Monitor {
	logger := log.With().Str("host", displayURL.Host).Str("container_id", containerID).Logger()
	isFirstFailure := true
	var state *healthcheck.DockerHealthcheckState
	var stateMu sync.Mutex

	var mon monitor
	mon.init(displayURL, config, func(_ *url.URL) (result Result, err error) {
		stateMu.Lock()
		defer stateMu.Unlock()

		if state == nil {
			client, clientErr := newClient()
			if clientErr != nil {
				err = fmt.Errorf("initialize docker health check: %w", clientErr)
			} else if client == nil {
				err = errors.New("initialize docker health check: client is nil")
			} else {
				state = healthcheck.NewDockerHealthcheckState(client, containerID)
			}
		}

		if err == nil {
			result, err = healthcheck.Docker(mon.Context(), state, config.Timeout)
		}
		if err == nil {
			return result, nil
		}
		if errors.Is(err, context.Canceled) {
			return Result{}, err
		}
		if errors.Is(err, healthcheck.ErrDockerHealthCheckNotAvailable) && !config.Disable && fallback != nil {
			if err := mon.Context().Err(); err != nil {
				return Result{}, err
			}
			return fallback.CheckHealth()
		}
		if isFirstFailure {
			isFirstFailure = false
			if !errors.Is(err, healthcheck.ErrDockerHealthCheckNotAvailable) {
				logger.Err(err).Msg("docker health check failed")
			}
		}
		return Result{Detail: err.Error()}, nil
	})
	if fallback != nil {
		mon.onUpdateURL = fallback.UpdateURL
	}
	return &mon
}

func NewAgentProxiedMonitor(config health.HealthCheckConfig, agent *agentpool.Agent, targetURL *url.URL) Monitor {
	var mon monitor
	mon.init(targetURL, config, func(u *url.URL) (result Result, err error) {
		return CheckHealthAgentProxied(agent, config.Timeout, u)
	})
	return &mon
}

func CheckHealthAgentProxied(agent *agentpool.Agent, timeout time.Duration, targetURL *url.URL) (Result, error) {
	switch {
	case targetURL == nil:
		return Result{Detail: "no url specified"}, nil
	case targetURL.Host == "":
		return Result{Detail: "no host specified"}, nil
	}

	query := url.Values{
		"scheme":  {targetURL.Scheme},
		"host":    {targetURL.Host},
		"path":    {targetURL.Path},
		"timeout": {strconv.FormatInt(timeout.Milliseconds(), 10)},
	}
	resp, err := agent.DoHealthCheck(timeout, query.Encode())
	result := Result{
		Healthy: resp.Healthy,
		Detail:  resp.Detail,
		Latency: resp.Latency,
	}
	return result, err
}
