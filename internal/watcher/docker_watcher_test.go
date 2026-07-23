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
	"github.com/yusing/goutils/task"
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
	newDockerWatcherClient = func(_ context.Context, cfg types.DockerProviderConfig) (*docker.SharedClient, error) {
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

func TestDockerWatcherReportsInitializationFailureBeforeReadiness(t *testing.T) {
	oldFactory := newDockerWatcherClient
	t.Cleanup(func() { newDockerWatcherClient = oldFactory })

	sentinel := errors.New("malformed docker endpoint")
	newDockerWatcherClient = func(context.Context, types.DockerProviderConfig) (*docker.SharedClient, error) {
		return nil, sentinel
	}

	stream := NewDockerWatcher(types.DockerProviderConfig{}).Watch(task.GetTestTask(t))
	require.ErrorIs(t, <-stream.Ready, sentinel)
	require.ErrorIs(t, <-stream.Errors, sentinel)
	_, eventsOpen := <-stream.Events
	require.False(t, eventsOpen)
}

func TestDockerWatcherReportsInitialConnectionFailure(t *testing.T) {
	oldFactory := newDockerWatcherClient
	oldCheck := dockerWatcherCheckConnection
	t.Cleanup(func() {
		newDockerWatcherClient = oldFactory
		dockerWatcherCheckConnection = oldCheck
	})

	newDockerWatcherClient = func(context.Context, types.DockerProviderConfig) (*docker.SharedClient, error) {
		return &docker.SharedClient{}, nil
	}
	dockerWatcherCheckConnection = func(context.Context, *docker.SharedClient) bool { return false }

	stream := NewDockerWatcher(types.DockerProviderConfig{}).Watch(task.GetTestTask(t))
	require.ErrorIs(t, <-stream.Ready, ErrDockerWatcherConnection)
	require.ErrorIs(t, <-stream.Errors, ErrDockerWatcherConnection)
}

func TestDockerWatcherReadinessDoesNotWaitForEventStreamHeaders(t *testing.T) {
	oldFactory := newDockerWatcherClient
	oldCheck := dockerWatcherCheckConnection
	t.Cleanup(func() {
		newDockerWatcherClient = oldFactory
		dockerWatcherCheckConnection = oldCheck
	})

	eventsRequested := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("API-Version", "1.51")
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/events"):
			close(eventsRequested)
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := types.DockerProviderConfig{URL: srv.URL}
	newDockerWatcherClient = func(ctx context.Context, cfg types.DockerProviderConfig) (*docker.SharedClient, error) {
		return docker.NewClient(ctx, cfg, true)
	}

	watcherTask := task.GetTestTask(t).Subtask("docker-watcher", false)
	t.Cleanup(func() { watcherTask.Finish(nil) })
	stream := NewDockerWatcher(cfg).Watch(watcherTask)

	select {
	case err := <-stream.Ready:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("watcher readiness waited for Docker event-stream headers")
	}

	select {
	case <-eventsRequested:
	case <-time.After(time.Second):
		t.Fatal("Docker event stream was not requested")
	}
}

func TestDockerWatcherReportsRejectedDockerAPI(t *testing.T) {
	oldFactory := newDockerWatcherClient
	t.Cleanup(func() { newDockerWatcherClient = oldFactory })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	cfg := types.DockerProviderConfig{URL: srv.URL}
	newDockerWatcherClient = func(ctx context.Context, cfg types.DockerProviderConfig) (*docker.SharedClient, error) {
		return docker.NewClient(ctx, cfg, true)
	}

	stream := NewDockerWatcher(cfg).Watch(task.GetTestTask(t))
	require.ErrorIs(t, <-stream.Ready, ErrDockerWatcherConnection)
	require.ErrorIs(t, <-stream.Errors, ErrDockerWatcherConnection)
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
	newDockerWatcherClient = func(_ context.Context, cfg types.DockerProviderConfig) (*docker.SharedClient, error) {
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
	newDockerWatcherClient = func(ctx context.Context, cfg types.DockerProviderConfig) (*docker.SharedClient, error) {
		clientCreations.Add(1)
		return docker.NewClient(ctx, cfg, true)
	}
	dockerWatcherCheckConnection = func(ctx context.Context, client *docker.SharedClient) bool {
		return true
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := NewDockerWatcher(cfg)
	stream := w.EventsWithOptions(ctx, optionsDefault)
	require.NoError(t, <-stream.Ready)

	select {
	case err := <-stream.Errors:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher error")
	}

	select {
	case ev := <-stream.Events:
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

	events := stream.Events
	errs := stream.Errors
	timeout := time.After(2 * time.Second)
	for events != nil || errs != nil {
		select {
		case _, ok := <-events:
			if !ok {
				events = nil
			}
		case _, ok := <-errs:
			if !ok {
				errs = nil
			}
		case <-timeout:
			t.Fatal("timed out waiting for watcher shutdown")
		}
	}
}
