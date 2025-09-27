package monitor

import (
	"github.com/docker/docker/api/types/container"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/types"
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

func (mon *DockerHealthMonitor) CheckHealth() (types.HealthCheckResult, error) {
	// if docker health check failed too many times, use fallback forever
	if mon.numDockerFailures > dockerFailuresThreshold {
		return mon.fallback.CheckHealth()
	}

	ctx, cancel := mon.ContextWithTimeout("docker health check timed out")
	defer cancel()

	cont, err := mon.client.ContainerInspect(ctx, mon.containerID)
	if err != nil {
		mon.numDockerFailures++
		return mon.fallback.CheckHealth()
	}

	status := cont.State.Status
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
	if cont.State.Health == nil { // no health check from docker, directly use fallback starting from next check
		mon.numDockerFailures = dockerFailuresThreshold + 1
		return mon.fallback.CheckHealth()
	}

	mon.numDockerFailures = 0
	result := types.HealthCheckResult{
		Healthy: cont.State.Health.Status == container.Healthy,
	}
	if len(cont.State.Health.Log) > 0 {
		lastLog := cont.State.Health.Log[len(cont.State.Health.Log)-1]
		result.Detail = lastLog.Output
		result.Latency = lastLog.End.Sub(lastLog.Start)
	}
	return result, nil
}
