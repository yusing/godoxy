package healthcheck

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/types"
	httputils "github.com/yusing/goutils/http"
	strutils "github.com/yusing/goutils/strings"
)

type DockerHealthcheckState struct {
	client      *docker.SharedClient
	containerID string

	numDockerFailures int
}

const dockerFailuresThreshold = 3

var (
	ErrDockerHealthCheckFailedTooManyTimes = errors.New("docker health check failed too many times")
	ErrDockerHealthCheckNotAvailable       = errors.New("docker health check not available")
	dockerHealthInspect                    = func(ctx context.Context, client *docker.SharedClient, containerID string) (container.State, error) {
		// the actual inspect response is intercepted and returned as RequestInterceptedError
		_, err := client.ContainerInspect(ctx, containerID)

		var interceptedErr *httputils.RequestInterceptedError
		if !httputils.AsRequestInterceptedError(err, &interceptedErr) {
			if err == nil {
				err = errors.New("inspect response was not intercepted")
			}
			return container.State{}, err
		}

		if interceptedErr == nil || interceptedErr.Data == nil { // should not happen
			return container.State{}, errors.New("intercepted error is nil or data is nil")
		}

		return interceptedErr.Data.(container.State), nil
	}
)

func NewDockerHealthcheckState(client *docker.SharedClient, containerID string) *DockerHealthcheckState {
	client.InterceptHTTPClient(interceptDockerInspectResponse)
	return &DockerHealthcheckState{
		client:            client,
		containerID:       containerID,
		numDockerFailures: 0,
	}
}

func Docker(ctx context.Context, state *DockerHealthcheckState, timeout time.Duration) (types.HealthCheckResult, error) {
	if state.numDockerFailures > dockerFailuresThreshold {
		return types.HealthCheckResult{}, ErrDockerHealthCheckFailedTooManyTimes
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	containerState, err := dockerHealthInspect(ctx, state.client, state.containerID)
	if err != nil {
		state.numDockerFailures++
		return types.HealthCheckResult{}, err
	}

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
		// no health check from docker, return error to trigger fallback
		state.numDockerFailures = dockerFailuresThreshold + 1
		return types.HealthCheckResult{}, ErrDockerHealthCheckNotAvailable
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
	err = strutils.UnmarshalJSON(body, &state)
	release(body)
	if err != nil {
		return false, err
	}

	return true, httputils.NewRequestInterceptedError(resp, state)
}
