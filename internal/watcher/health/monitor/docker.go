package monitor

import (
	"github.com/docker/docker/api/types/container"
	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/types"
)

type DockerHealthMonitor struct {
	*monitor
	client      *docker.SharedClient
	containerID string
	fallback    types.HealthChecker
}

func NewDockerHealthMonitor(client *docker.SharedClient, containerID, alias string, config *types.HealthCheckConfig, fallback types.HealthChecker) *DockerHealthMonitor {
	mon := new(DockerHealthMonitor)
	mon.client = client
	mon.containerID = containerID
	mon.monitor = newMonitor(fallback.URL(), config, mon.CheckHealth)
	mon.fallback = fallback
	mon.service = alias
	return mon
}

func (mon *DockerHealthMonitor) CheckHealth() (result *types.HealthCheckResult, err error) {
	ctx, cancel := mon.ContextWithTimeout("docker health check timed out")
	defer cancel()
	cont, err := mon.client.ContainerInspect(ctx, mon.containerID)
	if err != nil {
		return mon.fallback.CheckHealth()
	}
	status := cont.State.Status
	switch status {
	case "dead", "exited", "paused", "restarting", "removing":
		return &types.HealthCheckResult{
			Healthy: false,
			Detail:  "container is " + status,
		}, nil
	case "created":
		return &types.HealthCheckResult{
			Healthy: false,
			Detail:  "container is not started",
		}, nil
	}
	if cont.State.Health == nil {
		return mon.fallback.CheckHealth()
	}
	result = new(types.HealthCheckResult)
	result.Healthy = cont.State.Health.Status == container.Healthy
	if len(cont.State.Health.Log) > 0 {
		lastLog := cont.State.Health.Log[len(cont.State.Health.Log)-1]
		result.Detail = lastLog.Output
		result.Latency = lastLog.End.Sub(lastLog.Start)
	}
	return
}
