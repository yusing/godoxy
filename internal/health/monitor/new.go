package monitor

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/docker"
	healthcheck "github.com/yusing/godoxy/internal/health/check"
	"github.com/yusing/godoxy/internal/types"
)

type Result = types.HealthCheckResult
type Monitor = types.HealthMonCheck

// NewMonitor creates a health monitor based on the route type and configuration.
//
// See internal/health/monitor/README.md for detailed health check flow and conditions.
func NewMonitor(r types.Route) Monitor {
	target := &r.TargetURL().URL

	var mon Monitor
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

func NewHTTPHealthMonitor(config types.HealthCheckConfig, u *url.URL) Monitor {
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

func NewFileServerHealthMonitor(config types.HealthCheckConfig, path string) Monitor {
	var mon monitor
	mon.init(&url.URL{Scheme: "file", Host: path}, config, func(u *url.URL) (result Result, err error) {
		return healthcheck.FileServer(path)
	})
	return &mon
}

func NewStreamHealthMonitor(config types.HealthCheckConfig, targetUrl *url.URL) Monitor {
	var mon monitor
	mon.init(targetUrl, config, func(u *url.URL) (result Result, err error) {
		return healthcheck.Stream(mon.Context(), u, config.Timeout)
	})
	return &mon
}

func NewDockerHealthMonitor(config types.HealthCheckConfig, client *docker.SharedClient, containerId string, fallback Monitor) Monitor {
	state := healthcheck.NewDockerHealthcheckState(client, containerId)
	displayURL := &url.URL{ // only for display purposes, no actual request is made
		Scheme: "docker",
		Host:   client.DaemonHost(),
		Path:   "/containers/" + containerId + "/json",
	}
	logger := log.With().Str("host", client.DaemonHost()).Str("container_id", containerId).Logger()
	isFirstFailure := true

	var mon monitor
	mon.init(displayURL, config, func(_ *url.URL) (result Result, err error) {
		result, err = healthcheck.Docker(mon.Context(), state, config.Timeout)
		if err != nil {
			if isFirstFailure {
				isFirstFailure = false
				if !errors.Is(err, healthcheck.ErrDockerHealthCheckNotAvailable) {
					logger.Err(err).Msg("docker health check failed, using fallback")
				}
			}
			return fallback.CheckHealth()
		}
		return result, nil
	})
	mon.onUpdateURL = fallback.UpdateURL
	return &mon
}

func NewAgentProxiedMonitor(config types.HealthCheckConfig, agent *agentpool.Agent, targetUrl *url.URL) Monitor {
	var mon monitor
	mon.init(targetUrl, config, func(u *url.URL) (result Result, err error) {
		return CheckHealthAgentProxied(agent, config.Timeout, u)
	})
	return &mon
}

func CheckHealthAgentProxied(agent *agentpool.Agent, timeout time.Duration, targetUrl *url.URL) (Result, error) {
	query := url.Values{
		"scheme":  {targetUrl.Scheme},
		"host":    {targetUrl.Host},
		"path":    {targetUrl.Path},
		"timeout": {fmt.Sprintf("%d", timeout.Milliseconds())},
	}
	resp, err := agent.DoHealthCheck(timeout, query.Encode())
	result := Result{
		Healthy: resp.Healthy,
		Detail:  resp.Detail,
		Latency: resp.Latency,
	}
	return result, err
}
