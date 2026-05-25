package watcher

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dockerEvents "github.com/docker/docker/api/types/events"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/types"
)

func TestReceiveDockerStreamResultClosedMessageChannelReturnsStreamClosed(t *testing.T) {
	t.Parallel()

	msgCh := make(chan dockerEvents.Message)
	errCh := make(chan error)
	close(msgCh)

	result := receiveDockerStreamResult(context.Background(), msgCh, errCh)
	require.ErrorIs(t, result.err, ErrDockerEventStreamClosed)
	require.False(t, result.hasMessage)
	require.False(t, result.done)
}

func TestReceiveDockerStreamResultClosedErrorChannelReturnsStreamClosed(t *testing.T) {
	t.Parallel()

	msgCh := make(chan dockerEvents.Message)
	errCh := make(chan error)
	close(errCh)

	result := receiveDockerStreamResult(context.Background(), msgCh, errCh)
	require.ErrorIs(t, result.err, ErrDockerEventStreamClosed)
	require.False(t, result.hasMessage)
	require.False(t, result.done)
}

func TestReconnectDockerWatcherClientCreatesFreshClientPerRetry(t *testing.T) {
	oldFactory := newDockerWatcherClient
	oldCheck := dockerWatcherCheckConnection
	oldRetry := dockerWatcherRetryInterval
	t.Cleanup(func() {
		newDockerWatcherClient = oldFactory
		dockerWatcherCheckConnection = oldCheck
		dockerWatcherRetryInterval = oldRetry
	})
	dockerWatcherRetryInterval = time.Millisecond

	cfg := types.DockerProviderConfig{URL: "unix:///var/run/docker.sock"}
	var created []*docker.SharedClient
	newDockerWatcherClient = func(cfg types.DockerProviderConfig) (*docker.SharedClient, error) {
		c := &docker.SharedClient{}
		created = append(created, c)
		return c, nil
	}

	checkCalls := 0
	dockerWatcherCheckConnection = func(ctx context.Context, client *docker.SharedClient) bool {
		checkCalls++
		return checkCalls >= 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	errCh := make(chan error, 8)
	client, err := reconnectDockerWatcherClient(ctx, cfg, errCh)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Len(t, created, 2)
	require.Same(t, created[1], client)
	require.NotSame(t, created[0], created[1])
}

func TestReconnectDockerWatcherClientRetriesAfterFactoryError(t *testing.T) {
	oldFactory := newDockerWatcherClient
	oldCheck := dockerWatcherCheckConnection
	oldRetry := dockerWatcherRetryInterval
	t.Cleanup(func() {
		newDockerWatcherClient = oldFactory
		dockerWatcherCheckConnection = oldCheck
		dockerWatcherRetryInterval = oldRetry
	})
	dockerWatcherRetryInterval = time.Millisecond

	cfg := types.DockerProviderConfig{URL: "unix:///var/run/docker.sock"}
	factoryCalls := 0
	newDockerWatcherClient = func(cfg types.DockerProviderConfig) (*docker.SharedClient, error) {
		factoryCalls++
		if factoryCalls == 1 {
			return nil, errors.New("boom")
		}
		return &docker.SharedClient{}, nil
	}
	dockerWatcherCheckConnection = func(ctx context.Context, client *docker.SharedClient) bool {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	errCh := make(chan error, 8)
	client, err := reconnectDockerWatcherClient(ctx, cfg, errCh)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, 2, factoryCalls)

	select {
	case got := <-errCh:
		require.Error(t, got)
		require.Contains(t, got.Error(), "failed to reinitialize client")
	default:
		t.Fatal("expected reconnect error to be reported")
	}
}

func TestDockerWatcherReconnectsAfterEventStreamEOF(t *testing.T) {
	oldFactory := newDockerWatcherClient
	oldCheck := dockerWatcherCheckConnection
	oldRetry := dockerWatcherRetryInterval
	t.Cleanup(func() {
		newDockerWatcherClient = oldFactory
		dockerWatcherCheckConnection = oldCheck
		dockerWatcherRetryInterval = oldRetry
	})
	dockerWatcherRetryInterval = time.Millisecond

	var eventRequests atomic.Int32
	releaseSecondStream := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("API-Version", "1.51")
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/events"):
			if eventRequests.Add(1) == 1 {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusOK)
			<-releaseSecondStream
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := types.DockerProviderConfig{URL: srv.URL}
	var clientCreations atomic.Int32
	newDockerWatcherClient = func(cfg types.DockerProviderConfig) (*docker.SharedClient, error) {
		clientCreations.Add(1)
		return docker.NewClient(cfg, true)
	}
	dockerWatcherCheckConnection = func(ctx context.Context, client *docker.SharedClient) bool {
		return true
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := NewDockerWatcher(cfg)
	eventCh, errCh := w.EventsWithOptions(ctx, optionsDefault)

	select {
	case err := <-errCh:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher error")
	}

	select {
	case ev := <-eventCh:
		require.Equal(t, reloadTrigger, ev)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reload trigger")
	}

	require.GreaterOrEqual(t, clientCreations.Load(), int32(2))
	require.Eventually(t, func() bool {
		return eventRequests.Load() >= 2
	}, 2*time.Second, 10*time.Millisecond)

	close(releaseSecondStream)
	cancel()
}
