package healthcheck

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/health"
	httputils "github.com/yusing/goutils/http"
	strutils "github.com/yusing/goutils/strings"
)

type DockerHealthcheckState struct {
	client      *docker.SharedClient
	containerID string
}

var (
	ErrDockerContainerStateNotAvailable = errors.New("docker container state not available")
	ErrDockerHealthCheckNotAvailable    = errors.New("docker health check not available")
)

func NewDockerHealthcheckState(client *docker.SharedClient, containerID string) *DockerHealthcheckState {
	// Health monitors use unique clients, so intercepting the inspect response here
	// avoids decoding the rest of the large container-inspect payload on every poll.
	client.InterceptHTTPClient(interceptDockerInspectResponse)
	return &DockerHealthcheckState{
		client:      client,
		containerID: containerID,
	}
}

func Docker(ctx context.Context, state *DockerHealthcheckState, timeout time.Duration) (health.HealthCheckResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	containerState, err := dockerHealthInspect(ctx, state.client, state.containerID)
	if err != nil {
		return health.HealthCheckResult{}, fmt.Errorf("inspect docker container %q: %w", state.containerID, err)
	}

	status := containerState.Status
	switch status {
	case "dead", "exited", "paused", "restarting", "removing":
		return health.HealthCheckResult{
			Healthy: false,
			Detail:  "container is " + status,
		}, nil
	case "created":
		return health.HealthCheckResult{
			Healthy: false,
			Detail:  "container is not started",
		}, nil
	}

	dockerHealth := containerState.Health
	if dockerHealth == nil {
		return health.HealthCheckResult{}, ErrDockerHealthCheckNotAvailable
	}

	result := health.HealthCheckResult{
		Healthy: dockerHealth.Status == container.Healthy,
	}
	if len(dockerHealth.Log) > 0 {
		lastLog := dockerHealth.Log[len(dockerHealth.Log)-1]
		result.Detail = lastLog.Output
		result.Latency = lastLog.End.Sub(lastLog.Start)
	}
	return result, nil
}

func dockerHealthInspect(ctx context.Context, client *docker.SharedClient, containerID string) (container.State, error) {
	_, err := client.ContainerInspect(ctx, containerID)

	var interceptedErr *httputils.RequestInterceptedError
	if !httputils.AsRequestInterceptedError(err, &interceptedErr) {
		if err == nil {
			err = errors.New("inspect response was not intercepted")
		}
		return container.State{}, err
	}
	if interceptedErr == nil || interceptedErr.Data == nil {
		return container.State{}, ErrDockerContainerStateNotAvailable
	}
	state, ok := interceptedErr.Data.(container.State)
	if !ok {
		return container.State{}, fmt.Errorf("unexpected intercepted inspect state type %T", interceptedErr.Data)
	}
	return state, nil
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

	var inspect struct {
		State *container.State `json:"State"`
	}
	err = strutils.UnmarshalJSON(body, &inspect)
	release(body)
	if err != nil {
		return false, err
	}
	if inspect.State == nil {
		return true, ErrDockerContainerStateNotAvailable
	}

	return true, httputils.NewRequestInterceptedError(resp, *inspect.State)
}
