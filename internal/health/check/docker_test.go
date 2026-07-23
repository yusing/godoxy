package healthcheck

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/types"
)

func TestDockerHealthcheckReturnsUnhealthyForStoppedStates(t *testing.T) {
	tests := []struct {
		status string
		detail string
	}{
		{status: "dead", detail: "container is dead"},
		{status: "exited", detail: "container is exited"},
		{status: "paused", detail: "container is paused"},
		{status: "restarting", detail: "container is restarting"},
		{status: "removing", detail: "container is removing"},
		{status: "created", detail: "container is not started"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			state := newDockerHealthcheckState(t, http.StatusOK, fmt.Sprintf(`{"State":{"Status":%q}}`, tt.status))

			result, err := Docker(t.Context(), state, time.Second)
			require.NoError(t, err)
			require.Equal(t, health.HealthCheckResult{
				Healthy: false,
				Detail:  tt.detail,
			}, result)
		})
	}
}

func TestDockerHealthcheckReadsNestedHealthyState(t *testing.T) {
	state := newDockerHealthcheckState(t, http.StatusOK, `{
		"Id":"test",
		"State":{
			"Status":"running",
			"Health":{
				"Status":"healthy",
				"Log":[{
					"Start":"2026-07-21T00:00:00Z",
					"End":"2026-07-21T00:00:00.1Z",
					"ExitCode":0,
					"Output":"healthy"
				}]
			}
		}
	}`)

	result, err := Docker(t.Context(), state, time.Second)
	require.NoError(t, err)
	require.Equal(t, health.HealthCheckResult{
		Healthy: true,
		Detail:  "healthy",
		Latency: 100 * time.Millisecond,
	}, result)
}

func TestDockerHealthcheckIgnoresUnrelatedTopLevelCollision(t *testing.T) {
	state := newDockerHealthcheckState(t, http.StatusOK, `{
		"Status":"exited",
		"Health":{"Status":"unhealthy"},
		"State":{"Status":"running","Health":{"Status":"healthy"}}
	}`)

	result, err := Docker(t.Context(), state, time.Second)
	require.NoError(t, err)
	require.True(t, result.Healthy)
}

func TestDockerHealthcheckAllowsUnknownFutureFields(t *testing.T) {
	state := newDockerHealthcheckState(t, http.StatusOK, `{
		"FutureTopLevel":{"enabled":true},
		"State":{
			"Status":"running",
			"FutureStateField":"future-value",
			"Health":{"Status":"healthy","FutureHealthField":42}
		}
	}`)

	result, err := Docker(t.Context(), state, time.Second)
	require.NoError(t, err)
	require.True(t, result.Healthy)
}

func TestDockerHealthcheckReportsMissingHealth(t *testing.T) {
	state := newDockerHealthcheckState(t, http.StatusOK, `{"State":{"Status":"running"}}`)

	_, err := Docker(t.Context(), state, time.Second)
	require.ErrorIs(t, err, ErrDockerHealthCheckNotAvailable)
}

func TestDockerHealthcheckReportsMissingState(t *testing.T) {
	state := newDockerHealthcheckState(t, http.StatusOK, `{}`)

	_, err := Docker(t.Context(), state, time.Second)
	require.ErrorIs(t, err, ErrDockerContainerStateNotAvailable)
}

func TestDockerHealthcheckPropagatesMalformedResponse(t *testing.T) {
	state := newDockerHealthcheckState(t, http.StatusOK, `{"State":`)

	_, err := Docker(t.Context(), state, time.Second)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrDockerHealthCheckNotAvailable)
}

func TestDockerHealthcheckPropagatesInspectFailure(t *testing.T) {
	state := newDockerHealthcheckState(t, http.StatusInternalServerError, `{"message":"boom"}`)

	_, err := Docker(t.Context(), state, time.Second)
	require.ErrorContains(t, err, "inspect docker container \"test\"")
	require.ErrorContains(t, err, "boom")
}

func TestDockerHealthcheckHonorsCancellation(t *testing.T) {
	state := newDockerHealthcheckState(t, http.StatusOK, `{"State":{"Status":"running"}}`)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := Docker(ctx, state, time.Second)
	require.ErrorIs(t, err, context.Canceled)
}

func newDockerHealthcheckState(t *testing.T, status int, body string) *DockerHealthcheckState {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/_ping":
			w.Header().Set("API-Version", "1.44")
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/containers/test/json"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := docker.NewClient(t.Context(), types.DockerProviderConfig{URL: server.URL}, true)
	require.NoError(t, err)
	t.Cleanup(client.Close)

	return NewDockerHealthcheckState(client, "test")
}
