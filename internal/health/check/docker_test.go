package healthcheck

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/health"
)

func TestDockerHealthcheckReturnsUnhealthyForStoppedStates(t *testing.T) {
	states := []struct {
		name   string
		status string
		detail string
	}{
		{name: "dead", status: "dead", detail: "container is dead"},
		{name: "exited", status: "exited", detail: "container is exited"},
		{name: "paused", status: "paused", detail: "container is paused"},
		{name: "restarting", status: "restarting", detail: "container is restarting"},
		{name: "removing", status: "removing", detail: "container is removing"},
		{name: "created", status: "created", detail: "container is not started"},
	}

	for _, tc := range states {
		t.Run(tc.name, func(t *testing.T) {
			state := &DockerHealthcheckState{
				client:      &docker.SharedClient{},
				containerID: "test",
			}

			oldInspect := dockerHealthInspect
			t.Cleanup(func() { dockerHealthInspect = oldInspect })
			dockerHealthInspect = func(ctx context.Context, _ *docker.SharedClient, _ string) (container.State, error) {
				return container.State{Status: tc.status}, nil
			}

			result, err := Docker(t.Context(), state, time.Second)
			require.NoError(t, err)
			require.Equal(t, health.HealthCheckResult{
				Healthy: false,
				Detail:  tc.detail,
			}, result)
		})
	}
}

func TestDockerHealthcheckReturnsHealthyWhenContainerHealthy(t *testing.T) {
	state := &DockerHealthcheckState{
		client:      &docker.SharedClient{},
		containerID: "test",
	}

	oldInspect := dockerHealthInspect
	t.Cleanup(func() { dockerHealthInspect = oldInspect })
	dockerHealthInspect = func(ctx context.Context, _ *docker.SharedClient, _ string) (container.State, error) {
		start := time.Now()
		return container.State{
			Status: "running",
			Health: &container.Health{
				Status: container.Healthy,
				Log: []*container.HealthcheckResult{{
					Start:  start,
					End:    start.Add(100 * time.Millisecond),
					Output: "healthy",
				}},
			},
		}, nil
	}

	result, err := Docker(t.Context(), state, time.Second)
	require.NoError(t, err)
	require.True(t, result.Healthy)
	require.Equal(t, "healthy", result.Detail)
	require.Positive(t, result.Latency)
}

func TestDockerHealthcheckFallsBackWhenDockerHealthMissing(t *testing.T) {
	state := &DockerHealthcheckState{
		client:      &docker.SharedClient{},
		containerID: "test",
	}

	oldInspect := dockerHealthInspect
	t.Cleanup(func() { dockerHealthInspect = oldInspect })
	dockerHealthInspect = func(ctx context.Context, _ *docker.SharedClient, _ string) (container.State, error) {
		st := container.State{Status: "running", Health: nil}
		return st, nil
	}

	_, err := Docker(t.Context(), state, time.Second)
	require.ErrorIs(t, err, ErrDockerHealthCheckNotAvailable)
}

func TestDockerHealthcheckReturnsErrorAfterThreshold(t *testing.T) {
	state := &DockerHealthcheckState{
		client:            &docker.SharedClient{},
		containerID:       "test",
		numDockerFailures: dockerFailuresThreshold + 1,
	}

	_, err := Docker(t.Context(), state, time.Second)
	require.ErrorIs(t, err, ErrDockerHealthCheckFailedTooManyTimes)
}

func TestDockerHealthcheckPropagatesInspectFailure(t *testing.T) {
	state := &DockerHealthcheckState{
		client:      &docker.SharedClient{},
		containerID: "test",
	}

	oldInspect := dockerHealthInspect
	t.Cleanup(func() { dockerHealthInspect = oldInspect })
	dockerHealthInspect = func(ctx context.Context, _ *docker.SharedClient, _ string) (container.State, error) {
		return container.State{}, errors.New("boom")
	}

	_, err := Docker(t.Context(), state, time.Second)
	require.EqualError(t, err, "boom")
}
