package healthcheck

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/types"
)

func TestDockerHealthcheckReturnsUnhealthyForStoppedStates(t *testing.T) {
	t.Parallel()

	states := []struct {
		name   string
		status string
		detail string
	}{
		{name: "exited", status: "exited", detail: "container is exited"},
		{name: "paused", status: "paused", detail: "container is paused"},
		{name: "created", status: "created", detail: "container is not started"},
	}

	for _, tc := range states {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := &DockerHealthcheckState{
				client:      &docker.SharedClient{},
				containerID: "test",
			}

			oldInspect := dockerHealthInspect
			t.Cleanup(func() { dockerHealthInspect = oldInspect })
			dockerHealthInspect = func(ctx context.Context, _ *docker.SharedClient, _ string) (container.State, error) {
				return container.State{Status: tc.status}, nil
			}

			result, err := Docker(context.Background(), state, time.Second)
			require.NoError(t, err)
			require.Equal(t, types.HealthCheckResult{
				Healthy: false,
				Detail:  tc.detail,
			}, result)
		})
	}
}

func TestDockerHealthcheckFallsBackWhenDockerHealthMissing(t *testing.T) {
	t.Parallel()

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

	_, err := Docker(context.Background(), state, time.Second)
	require.ErrorIs(t, err, ErrDockerHealthCheckNotAvailable)
}

func TestDockerHealthcheckPropagatesInspectFailure(t *testing.T) {
	t.Parallel()

	state := &DockerHealthcheckState{
		client:      &docker.SharedClient{},
		containerID: "test",
	}

	oldInspect := dockerHealthInspect
	t.Cleanup(func() { dockerHealthInspect = oldInspect })
	dockerHealthInspect = func(ctx context.Context, _ *docker.SharedClient, _ string) (container.State, error) {
		return container.State{}, errors.New("boom")
	}

	_, err := Docker(context.Background(), state, time.Second)
	require.EqualError(t, err, "boom")
}
