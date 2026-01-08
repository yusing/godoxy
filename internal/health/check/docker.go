package healthcheck

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/bytedance/sonic"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/types"
	httputils "github.com/yusing/goutils/http"
)

type DockerHealthcheckState struct {
	client      *docker.SharedClient
	containerId string

	numDockerFailures int
}

const dockerFailuresThreshold = 3

var errDockerHealthCheckFailedTooManyTimes = errors.New("docker health check failed too many times")

func NewDockerHealthcheckState(client *docker.SharedClient, containerId string) *DockerHealthcheckState {
	client.InterceptHTTPClient(interceptDockerInspectResponse)
	return &DockerHealthcheckState{
		client:            client,
		containerId:       containerId,
		numDockerFailures: 0,
	}
}

func Docker(ctx context.Context, state *DockerHealthcheckState, containerId string, timeout time.Duration) (types.HealthCheckResult, error) {
	if state.numDockerFailures > dockerFailuresThreshold {
		return types.HealthCheckResult{}, errDockerHealthCheckFailedTooManyTimes
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// the actual inspect response is intercepted and returned as RequestInterceptedError
	_, err := state.client.ContainerInspect(ctx, containerId, client.ContainerInspectOptions{})

	var interceptedErr *httputils.RequestInterceptedError
	if !httputils.AsRequestInterceptedError(err, &interceptedErr) {
		state.numDockerFailures++
		return types.HealthCheckResult{}, err
	}

	if interceptedErr == nil || interceptedErr.Data == nil { // should not happen
		state.numDockerFailures++
		return types.HealthCheckResult{}, errors.New("intercepted error is nil or data is nil")
	}

	containerState := interceptedErr.Data.(container.State)

	status := containerState.Status
	switch status {
	case "dead", "exited", "paused", "restarting", "removing":
		state.numDockerFailures = 0
		return types.HealthCheckResult{
			Healthy: false,
			Detail:  "container is " + string(status),
		}, nil
	case "created":
		state.numDockerFailures = 0
		return types.HealthCheckResult{
			Healthy: false,
			Detail:  "container is not started",
		}, nil
	}

	health := containerState.Health
	if health == nil {
		// no health check from docker, directly use fallback
		state.numDockerFailures = dockerFailuresThreshold + 1
		return types.HealthCheckResult{}, errDockerHealthCheckFailedTooManyTimes
	}

	state.numDockerFailures = 0
	result := types.HealthCheckResult{
		Healthy: health.Status == container.Healthy,
	}
	if len(health.Log) > 0 {
		lastLog := health.Log[len(health.Log)-1]
		result.Detail = lastLog.Output
		result.Latency = lastLog.End.Sub(lastLog.Start)
	}
	return result, nil
}

func interceptDockerInspectResponse(resp *http.Response) (intercepted bool, err error) {
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
