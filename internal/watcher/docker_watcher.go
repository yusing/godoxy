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
)

type (
	DockerWatcher struct {
		cfg types.DockerProviderConfig
	}
	DockerListOptions = dockerEvents.ListOptions
)

// https://docs.docker.com/reference/api/engine/version/v1.47/#tag/System/operation/SystemPingHead
var (
	DockerFilterContainer = filters.Arg("type", string(dockerEvents.ContainerEventType))
	DockerFilterStart     = filters.Arg("event", string(dockerEvents.ActionStart))
	DockerFilterStop      = filters.Arg("event", string(dockerEvents.ActionStop))
	DockerFilterDie       = filters.Arg("event", string(dockerEvents.ActionDie))
	DockerFilterDestroy   = filters.Arg("event", string(dockerEvents.ActionDestroy))
	DockerFilterKill      = filters.Arg("event", string(dockerEvents.ActionKill))
	DockerFilterPause     = filters.Arg("event", string(dockerEvents.ActionPause))
	DockerFilterUnpause   = filters.Arg("event", string(dockerEvents.ActionUnPause))

	NewDockerFilters = filters.NewArgs

	optionsDefault = DockerListOptions{Filters: NewDockerFilters(
		DockerFilterContainer,
		DockerFilterStart,
		// DockerFilterStop,
		DockerFilterDie,
		DockerFilterDestroy,
	)}

	dockerWatcherRetryInterval = 3 * time.Second

	reloadTrigger = Event{
		Type:            watcherEvents.EventTypeDocker,
		Action:          watcherEvents.ActionForceReload,
		ActorAttributes: map[string]string{},
		ActorName:       "",
		ActorID:         "",
	}
)

func DockerFilterContainerNameID(nameOrID string) filters.KeyValuePair {
	return filters.Arg("container", nameOrID)
}

func NewDockerWatcher(dockerCfg types.DockerProviderConfig) DockerWatcher {
	return DockerWatcher{
		cfg: dockerCfg,
	}
}

var _ Watcher = (*DockerWatcher)(nil)

// Events implements the Watcher interface.
func (w DockerWatcher) Events(ctx context.Context) (<-chan Event, <-chan error) {
	return w.EventsWithOptions(ctx, optionsDefault)
}

func (w DockerWatcher) EventsWithOptions(ctx context.Context, options DockerListOptions) (<-chan Event, <-chan error) {
	eventCh := make(chan Event)
	errCh := make(chan error)

	go func() {
		client, err := docker.NewClient(w.cfg)
		if err != nil {
			errCh <- fmt.Errorf("docker watcher: failed to initialize client: %w", err)
			return
		}

		defer func() {
			close(eventCh)
			close(errCh)
			client.Close()
		}()

		cEventCh, cErrCh := client.Events(ctx, options)
		defer log.Debug().Str("host", client.DaemonHost()).Msg("docker watcher closed")
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-cEventCh:
				w.handleEvent(msg, eventCh)
			case err := <-cErrCh:
				if err == nil {
					continue
				}
				errCh <- w.parseError(err)
				// release the error because reopening event channel may block
				//nolint:ineffassign,wastedassign
				err = nil
				// trigger reload (clear routes)
				eventCh <- reloadTrigger

				retry := time.NewTicker(dockerWatcherRetryInterval)
			outer:
				for {
					select {
					case <-ctx.Done():
						retry.Stop()
						return
					case <-retry.C:
						if checkConnection(ctx, client) {
							break outer
						}
					}
				}
				retry.Stop()
				// connection successful, trigger reload (reload routes)
				eventCh <- reloadTrigger
				// reopen event channel
				cEventCh, cErrCh = client.Events(ctx, options)
			}
		}
	}()

	return eventCh, errCh
}

func (w DockerWatcher) parseError(err error) error {
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
	err := client.CheckConnection(ctx)
	if err != nil {
		log.Debug().Err(err).Str("host", client.DaemonHost()).Msg("docker watcher: connection failed")
		return false
	}
	return true
}
