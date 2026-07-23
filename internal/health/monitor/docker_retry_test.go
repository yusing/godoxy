package monitor

import (
	"context"
	"errors"
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
	"github.com/yusing/godoxy/internal/types"
)

func TestDockerHealthMonitorRetriesClientInitializationWithoutFallback(t *testing.T) {
	dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/_ping":
			w.Header().Set("API-Version", "1.44")
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/containers/test/json"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"State":{"Status":"running","Health":{"Status":"healthy"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(dockerServer.Close)

	client, err := docker.NewClient(t.Context(), types.DockerProviderConfig{URL: dockerServer.URL}, true)
	require.NoError(t, err)
	t.Cleanup(client.Close)

	var fallbackRequests atomic.Int32
	fallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(fallbackServer.Close)
	fallbackURL, err := url.Parse(fallbackServer.URL)
	require.NoError(t, err)

	config := health.HealthCheckConfig{Interval: time.Second, Timeout: time.Second}
	fallback := NewHTTPHealthMonitor(config, fallbackURL)
	var attempts atomic.Int32
	mon := newDockerHealthMonitor(
		config,
		&url.URL{Scheme: "docker", Host: dockerServer.URL, Path: "/containers/test/json"},
		"test",
		fallback,
		func(context.Context) (*docker.SharedClient, error) {
			if attempts.Add(1) == 1 {
				return nil, errors.New("temporarily unavailable")
			}
			return client, nil
		},
	)

	first, err := mon.CheckHealth()
	require.NoError(t, err)
	require.False(t, first.Healthy)
	require.Contains(t, first.Detail, "initialize docker health check: temporarily unavailable")
	require.Zero(t, fallbackRequests.Load())

	second, err := mon.CheckHealth()
	require.NoError(t, err)
	require.True(t, second.Healthy)
	require.EqualValues(t, 2, attempts.Load())
	require.Zero(t, fallbackRequests.Load())
}
