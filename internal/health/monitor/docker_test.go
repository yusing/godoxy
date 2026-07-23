package monitor_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/health/monitor"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routeimpl"
	"github.com/yusing/godoxy/internal/types"
)

func TestDockerHealthMonitorUsesDockerWithoutFallback(t *testing.T) {
	client := newDockerClient(t, func() (int, string) {
		return http.StatusOK, healthyDockerInspectResponse
	})
	fallback, fallbackRequests := newHTTPFallback(t)

	mon := monitor.NewDockerHealthMonitor(testHealthConfig(false), client, "test", fallback)
	result, err := mon.CheckHealth()

	require.NoError(t, err)
	require.True(t, result.Healthy)
	require.Zero(t, fallbackRequests.Load())
}

func TestDockerHealthMonitorFallsBackWhenDockerHealthIsUnavailable(t *testing.T) {
	client := newDockerClient(t, func() (int, string) {
		return http.StatusOK, `{"State":{"Status":"running"}}`
	})
	fallback, fallbackRequests := newHTTPFallback(t)

	mon := monitor.NewDockerHealthMonitor(testHealthConfig(false), client, "test", fallback)
	result, err := mon.CheckHealth()

	require.NoError(t, err)
	require.True(t, result.Healthy)
	require.EqualValues(t, 1, fallbackRequests.Load())
}

func TestDockerHealthMonitorSuppressesDisabledFallbackAndRetriesDocker(t *testing.T) {
	var inspectRequests atomic.Int32
	client := newDockerClient(t, func() (int, string) {
		if inspectRequests.Add(1) == 1 {
			return http.StatusOK, `{"State":{"Status":"running"}}`
		}
		return http.StatusOK, healthyDockerInspectResponse
	})
	fallback, fallbackRequests := newHTTPFallback(t)

	mon := monitor.NewDockerHealthMonitor(testHealthConfig(true), client, "test", fallback)
	first, err := mon.CheckHealth()
	require.NoError(t, err)
	require.False(t, first.Healthy)
	require.Equal(t, "docker health check not available", first.Detail)
	require.Zero(t, fallbackRequests.Load())

	second, err := mon.CheckHealth()
	require.NoError(t, err)
	require.True(t, second.Healthy)
	require.Zero(t, fallbackRequests.Load())
	require.EqualValues(t, 2, inspectRequests.Load())
}

func TestDockerHealthMonitorDoesNotFallbackOnMalformedResponseWhenDisabled(t *testing.T) {
	client := newDockerClient(t, func() (int, string) {
		return http.StatusOK, `{"State":`
	})
	fallback, fallbackRequests := newHTTPFallback(t)

	mon := monitor.NewDockerHealthMonitor(testHealthConfig(true), client, "test", fallback)
	result, err := mon.CheckHealth()

	require.NoError(t, err)
	require.False(t, result.Healthy)
	require.Contains(t, result.Detail, "inspect docker container \"test\"")
	require.Zero(t, fallbackRequests.Load())
}

func TestDockerHealthMonitorDoesNotMaskMalformedResponseWithFallback(t *testing.T) {
	client := newDockerClient(t, func() (int, string) {
		return http.StatusOK, `{"State":`
	})
	fallback, fallbackRequests := newHTTPFallback(t)

	mon := monitor.NewDockerHealthMonitor(testHealthConfig(false), client, "test", fallback)
	result, err := mon.CheckHealth()

	require.NoError(t, err)
	require.False(t, result.Healthy)
	require.Contains(t, result.Detail, "inspect docker container \"test\"")
	require.Zero(t, fallbackRequests.Load())
}

func TestDockerHealthMonitorCancellationDoesNotStartFallback(t *testing.T) {
	inspectStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/_ping":
			w.Header().Set("API-Version", "1.44")
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/containers/test/json"):
			close(inspectStarted)
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := docker.NewClient(t.Context(), types.DockerProviderConfig{URL: server.URL}, true)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	fallback, fallbackRequests := newHTTPFallback(t)

	ctx, cancel := context.WithCancel(t.Context())
	config := testHealthConfig(false)
	config.BaseContext = func() context.Context { return ctx }
	mon := monitor.NewDockerHealthMonitor(config, client, "test", fallback)

	done := make(chan error, 1)
	go func() {
		_, err := mon.CheckHealth()
		done <- err
	}()
	<-inspectStarted
	cancel()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("Docker health monitor did not return after cancellation")
	}
	require.Zero(t, fallbackRequests.Load())
}

func TestNewMonitorDoesNotProbeWhenDockerClientInitializationFails(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("disabled=%v", disabled), func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("unexpected fallback probe")
			}))
			t.Cleanup(backend.Close)
			targetURL, err := nettypes.ParseURL(backend.URL)
			require.NoError(t, err)

			base := &route.Route{
				Alias:       "test",
				Scheme:      route.SchemeHTTP,
				HealthCheck: testHealthConfig(disabled),
				Metadata: route.Metadata{
					ProxyURL: targetURL,
					Container: &docker.Container{
						ContainerID:        "test",
						HealthCheckEnabled: true,
						DockerCfg: types.DockerProviderConfig{
							URL: "https://127.0.0.1:2376",
						},
						Running: true,
					},
				},
			}
			reverseProxyRoute, err := routeimpl.NewReverseProxyRoute(base)
			require.NoError(t, err)

			mon := monitor.NewMonitor(reverseProxyRoute)
			result, err := mon.CheckHealth()

			require.NoError(t, err)
			require.False(t, result.Healthy)
			require.Contains(t, result.Detail, "initialize docker health check")
			require.Contains(t, result.Detail, "TLS config is not set")
		})
	}
}

func TestNewMonitorSkipsDockerWhenHealthCheckWasNotEnabledAtLoad(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)
	targetURL, err := nettypes.ParseURL(backend.URL)
	require.NoError(t, err)

	base := &route.Route{
		Alias:       "test",
		Scheme:      route.SchemeHTTP,
		HealthCheck: testHealthConfig(false),
		Metadata: route.Metadata{
			ProxyURL: targetURL,
			Container: &docker.Container{
				ContainerID: "test",
				DockerCfg: types.DockerProviderConfig{
					URL: "https://127.0.0.1:2376",
				},
				Running: true,
			},
		},
	}
	reverseProxyRoute, err := routeimpl.NewReverseProxyRoute(base)
	require.NoError(t, err)

	result, err := monitor.NewMonitor(reverseProxyRoute).CheckHealth()
	require.NoError(t, err)
	require.True(t, result.Healthy)
}

const healthyDockerInspectResponse = `{
	"State":{"Status":"running","Health":{"Status":"healthy"}}
}`

func testHealthConfig(disable bool) health.HealthCheckConfig {
	return health.HealthCheckConfig{
		Disable:  disable,
		Interval: time.Second,
		Timeout:  time.Second,
		Path:     "/",
	}
}

func newDockerClient(t *testing.T, response func() (status int, body string)) *docker.SharedClient {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/_ping":
			w.Header().Set("API-Version", "1.44")
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/containers/test/json"):
			status, body := response()
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
	return client
}

func newHTTPFallback(t *testing.T) (monitor.Monitor, *atomic.Int32) {
	t.Helper()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	targetURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	return monitor.NewHTTPHealthMonitor(testHealthConfig(false), targetURL), &requests
}
