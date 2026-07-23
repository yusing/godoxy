package watcher

import (
	"context"
	"errors"
	"fmt"
	"time"

	dockerEvents "github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/types"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/task"
)

type (
	DockerWatcher struct {
		cfg types.DockerProviderConfig
	}
	DockerListOptions = dockerEvents.ListOptions

	dockerStreamResult struct {
		message    dockerEvents.Message
		hasMessage bool
		err        error
		done       bool
	}
)

type DockerFilter = filters.KeyValuePair

var (
	NewDockerFilter        = filters.Arg
	NewDockerFilters       = filters.NewArgs
	newDockerWatcherClient = func(ctx context.Context, cfg types.DockerProviderConfig) (*docker.SharedClient, error) {
		return docker.NewClient(ctx, cfg, true)
	}
	dockerWatcherCheckConnection = checkConnection
)

// https://docs.docker.com/reference/api/engine/version/v1.47/#tag/System/operation/SystemPingHead
var (
	DockerFilterContainer = NewDockerFilter("type", string(dockerEvents.ContainerEventType))
	DockerFilterStart     = NewDockerFilter("event", string(dockerEvents.ActionStart))
	DockerFilterStop      = NewDockerFilter("event", string(dockerEvents.ActionStop))
	DockerFilterDie       = NewDockerFilter("event", string(dockerEvents.ActionDie))
	DockerFilterDestroy   = NewDockerFilter("event", string(dockerEvents.ActionDestroy))
	DockerFilterKill      = NewDockerFilter("event", string(dockerEvents.ActionKill))
	DockerFilterPause     = NewDockerFilter("event", string(dockerEvents.ActionPause))
	DockerFilterUnpause   = NewDockerFilter("event", string(dockerEvents.ActionUnPause))

	optionsDefault = DockerListOptions{Filters: NewDockerFilters(
		DockerFilterContainer,
		DockerFilterStart,
		// DockerFilterStop,
		DockerFilterDie,
		DockerFilterDestroy,
	)}

	dockerWatcherRetryInterval = 3 * time.Second
	ErrDockerEventStreamClosed = errors.New("docker watcher: event stream closed")
	ErrDockerWatcherConnection = errors.New("docker watcher: initial connection failed")

	reloadTrigger = Event{
		Type:            watcherEvents.EventTypeDocker,
		Action:          watcherEvents.ActionForceReload,
		ActorAttributes: map[string]string{},
		ActorName:       "",
		ActorID:         "",
	}
)

func DockerFilterContainerNameID(nameOrID string) DockerFilter {
	return NewDockerFilter("container", nameOrID)
}

func NewDockerWatcher(dockerCfg types.DockerProviderConfig) DockerWatcher {
	return DockerWatcher{
		cfg: dockerCfg,
	}
}

var _ Watcher = (*DockerWatcher)(nil)

// Watch implements the Watcher interface.
func (w DockerWatcher) Watch(parent task.Parent) Stream {
	return w.EventsWithOptions(parent.Context(), optionsDefault)
}

func (w DockerWatcher) EventsWithOptions(ctx context.Context, options DockerListOptions) Stream {
	eventCh := make(chan Event)
	errCh := make(chan error, 1)
	readyCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		signalReady := func(err error) {
			readyCh <- err
			close(readyCh)
		}
		initializationFailed := func(err error) {
			signalReady(err)
			errCh <- err
		}
		client, err := newDockerWatcherClient(ctx, w.cfg)
		if err != nil {
			initializationFailed(fmt.Errorf("docker watcher: failed to initialize client: %w", err))
			return
		}
		if !dockerWatcherCheckConnection(ctx, client) {
			client.Close()
			initializationFailed(ErrDockerWatcherConnection)
			return
		}

		defer func() {
			if client != nil {
				client.Close()
			}
		}()

		// docker.Client.Events blocks until the daemon (or an intermediary
		// proxy) sends the event-stream response headers. Some valid proxies do
		// not flush those headers until the first event, so waiting for Events
		// to return would make runtime activation depend on unrelated Docker
		// activity. The bounded connection check above is the initialization
		// boundary; stream failures after it are handled by the reconnect loop.
		signalReady(nil)
		msgCh, dErrCh := client.Events(ctx, options)
		defer log.Debug().Str("host", client.DaemonHost()).Msg("docker watcher closed")
		for {
			result := receiveDockerStreamResult(ctx, msgCh, dErrCh)
			if result.done {
				return
			}
			if result.hasMessage {
				w.handleEvent(result.message, eventCh)
				continue
			}

			if result.err == nil {
				continue
			}

			errCh <- w.parseError(result.err)
			client.Close()
			client, err = reconnectDockerWatcherClient(ctx, w.cfg, errCh)
			if err != nil {
				return
			}
			// connection successful, trigger reload so routes can be refreshed from the
			// latest container state without dropping the last known-good routes while the
			// daemon was temporarily unreachable.
			eventCh <- reloadTrigger
			// reopen event channel
			msgCh, dErrCh = client.Events(ctx, options)
		}
	}()

	return Stream{Events: eventCh, Errors: errCh, Ready: readyCh}
}

func receiveDockerStreamResult(ctx context.Context, msgCh <-chan dockerEvents.Message, streamErrCh <-chan error) dockerStreamResult {
	select {
	case <-ctx.Done():
		return dockerStreamResult{done: true}
	case msg, ok := <-msgCh:
		if !ok {
			return dockerStreamResult{err: ErrDockerEventStreamClosed}
		}
		return dockerStreamResult{
			message:    msg,
			hasMessage: true,
		}
	case err, ok := <-streamErrCh:
		if !ok {
			return dockerStreamResult{err: ErrDockerEventStreamClosed}
		}
		return dockerStreamResult{err: err}
	}
}

func (w DockerWatcher) parseError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrDockerEventStreamClosed) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("docker client connection timeout")
	}
	if client.IsErrConnectionFailed(err) {
		return errors.New("docker client connection failure")
	}
	return err
}

func (w DockerWatcher) handleEvent(event dockerEvents.Message, ch chan<- Event) {
	action, ok := watcherEvents.DockerEventMap[event.Action]
	if !ok {
		return
	}
	ch <- Event{
		Type:            watcherEvents.EventTypeDocker,
		ActorID:         event.Actor.ID,
		ActorAttributes: event.Actor.Attributes, // labels
		ActorName:       event.Actor.Attributes["name"],
		Action:          action,
	}
}

func checkConnection(ctx context.Context, client *docker.SharedClient) bool {
	ctx, cancel := context.WithTimeout(ctx, dockerWatcherRetryInterval)
	defer cancel()
	_, err := client.Ping(ctx)
	if err != nil {
		log.Debug().Err(err).Str("host", client.DaemonHost()).Msg("docker watcher: API check failed")
		return false
	}
	return true
}

func reconnectDockerWatcherClient(ctx context.Context, cfg types.DockerProviderConfig, errCh chan<- error) (*docker.SharedClient, error) {
	retry := time.NewTicker(dockerWatcherRetryInterval)
	defer retry.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-retry.C:
			client, err := newDockerWatcherClient(ctx, cfg)
			if err != nil {
				select {
				case errCh <- fmt.Errorf("docker watcher: failed to reinitialize client: %w", err):
				default:
				}
				continue
			}
			if !dockerWatcherCheckConnection(ctx, client) {
				client.Close()
				continue
			}
			return client, nil
		}
	}
}
