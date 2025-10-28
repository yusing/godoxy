package monitor

import (
	"net/http"

	"github.com/bytedance/sonic"
	"github.com/docker/docker/api/types/container"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/task"
)

type DockerHealthMonitor struct {
	*monitor
	client      *docker.SharedClient
	containerID string
	fallback    types.HealthChecker

	numDockerFailures int
}

const dockerFailuresThreshold = 3

func NewDockerHealthMonitor(client *docker.SharedClient, containerID, alias string, config *types.HealthCheckConfig, fallback types.HealthChecker) *DockerHealthMonitor {
	mon := new(DockerHealthMonitor)
	mon.client = client
	mon.containerID = containerID
	mon.monitor = newMonitor(fallback.URL(), config, mon.CheckHealth)
	mon.fallback = fallback
	mon.service = alias
	return mon
}

func (mon *DockerHealthMonitor) Start(parent task.Parent) gperr.Error {
	mon.client = mon.client.CloneUnique()
	err := mon.monitor.Start(parent)
	if err != nil {
		return err
	}
	// zero port
	if mon.monitor.task == nil {
		return nil
	}
	mon.client.InterceptHTTPClient(mon.interceptInspectResponse)
	mon.monitor.task.OnFinished("close docker client", mon.client.Close)
	return nil
}

func (mon *DockerHealthMonitor) interceptInspectResponse(resp *http.Response) (intercepted bool, err error) {
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	body, release, err := httputils.ReadAllBody(resp)
	resp.Body.Close()
	if err != nil {
		return false, err
	}

	var state container.State
	err = sonic.Unmarshal(body, &state)
	release(body)
	if err != nil {
		return false, err
	}

	return true, httputils.NewRequestInterceptedError(resp, state)
}

func (mon *DockerHealthMonitor) CheckHealth() (types.HealthCheckResult, error) {
	// if docker health check failed too many times, use fallback forever
	if mon.numDockerFailures > dockerFailuresThreshold {
		return mon.fallback.CheckHealth()
	}

	ctx, cancel := mon.ContextWithTimeout("docker health check timed out")
	defer cancel()

	// the actual inspect response is intercepted and returned as RequestInterceptedError
	_, err := mon.client.ContainerInspect(ctx, mon.containerID)

	var interceptedErr *httputils.RequestInterceptedError
	if !httputils.AsRequestInterceptedError(err, &interceptedErr) {
		mon.numDockerFailures++
		log.Debug().Err(err).Str("container_id", mon.containerID).Msg("docker health check failed, using fallback")
		return mon.fallback.CheckHealth()
	}

	if interceptedErr == nil || interceptedErr.Data == nil { // should not happen
		log.Debug().Msgf("intercepted error is nil or data is nil, container_id: %s", mon.containerID)
		mon.numDockerFailures++
		log.Debug().Err(err).Str("container_id", mon.containerID).Msg("docker health check failed, using fallback")
		return mon.fallback.CheckHealth()
	}

	state := interceptedErr.Data.(container.State)
	status := state.Status
	switch status {
	case "dead", "exited", "paused", "restarting", "removing":
		mon.numDockerFailures = 0
		return types.HealthCheckResult{
			Healthy: false,
			Detail:  "container is " + status,
		}, nil
	case "created":
		mon.numDockerFailures = 0
		return types.HealthCheckResult{
			Healthy: false,
			Detail:  "container is not started",
		}, nil
	}
	if state.Health == nil { // no health check from docker, directly use fallback starting from next check
		mon.numDockerFailures = dockerFailuresThreshold + 1
		return mon.fallback.CheckHealth()
	}

	mon.numDockerFailures = 0
	result := types.HealthCheckResult{
		Healthy: state.Health.Status == container.Healthy,
	}
	if len(state.Health.Log) > 0 {
		lastLog := state.Health.Log[len(state.Health.Log)-1]
		result.Detail = lastLog.Output
		result.Latency = lastLog.End.Sub(lastLog.Start)
	}
	return result, nil
}
