package watcher

import (
	"context"
	"errors"
	"time"

	dockerEvents "github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

type (
	DockerWatcher     string
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

	NewDockerFilter = filters.NewArgs

	optionsDefault = DockerListOptions{Filters: NewDockerFilter(
		DockerFilterContainer,
		DockerFilterStart,
		// DockerFilterStop,
		DockerFilterDie,
		DockerFilterDestroy,
	)}

	dockerWatcherRetryInterval = 3 * time.Second

	reloadTrigger = Event{
		Type:            events.EventTypeDocker,
		Action:          events.ActionForceReload,
		ActorAttributes: map[string]string{},
		ActorName:       "",
		ActorID:         "",
	}
)

func DockerFilterContainerNameID(nameOrID string) filters.KeyValuePair {
	return filters.Arg("container", nameOrID)
}

func NewDockerWatcher(host string) DockerWatcher {
	return DockerWatcher(host)
}

func (w DockerWatcher) Events(ctx context.Context) (<-chan Event, <-chan gperr.Error) {
	return w.EventsWithOptions(ctx, optionsDefault)
}

func (w DockerWatcher) EventsWithOptions(ctx context.Context, options DockerListOptions) (<-chan Event, <-chan gperr.Error) {
	eventCh := make(chan Event)
	errCh := make(chan gperr.Error)

	go func() {
		client, err := docker.NewClient(string(w))
		if err != nil {
			errCh <- gperr.Wrap(err, "docker watcher: failed to initialize client")
			return
		}

		defer func() {
			close(eventCh)
			close(errCh)
			client.Close()
		}()

		cEventCh, cErrCh := client.Events(ctx, options)
		defer log.Debug().Str("host", client.Address()).Msg("docker watcher closed")
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
				defer retry.Stop()
				ok := false
			outer:
				for !ok {
					select {
					case <-ctx.Done():
						return
					case <-retry.C:
						if checkConnection(ctx, client) {
							ok = true
							break outer
						}
					}
				}
				// connection successful, trigger reload (reload routes)
				eventCh <- reloadTrigger
				// reopen event channel
				cEventCh, cErrCh = client.Events(ctx, options)
			}
		}
	}()

	return eventCh, errCh
}

func (w DockerWatcher) parseError(err error) gperr.Error {
	if errors.Is(err, context.DeadlineExceeded) {
		return gperr.New("docker client connection timeout")
	}
	if client.IsErrConnectionFailed(err) {
		return gperr.New("docker client connection failure")
	}
	return gperr.Wrap(err)
}

func (w DockerWatcher) handleEvent(event dockerEvents.Message, ch chan<- Event) {
	action, ok := events.DockerEventMap[event.Action]
	if !ok {
		return
	}
	ch <- Event{
		Type:            events.EventTypeDocker,
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
		log.Debug().Err(err).Msg("docker watcher: connection failed")
		return false
	}
	return true
}
