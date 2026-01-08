package watcher

import (
	"context"
	"errors"
	"time"

	dockerEvents "github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/godoxy/internal/watcher/events"
	gperr "github.com/yusing/goutils/errs"
)

type (
	DockerWatcher struct {
		cfg types.DockerProviderConfig
	}
	DockerListOptions = client.EventsListOptions
	DockerFilters     = client.Filters
)

type DockerFilter struct {
	Term   string
	Values []string
}

func NewDockerFilter(term string, values ...string) DockerFilter {
	return DockerFilter{
		Term:   term,
		Values: values,
	}
}

func NewDockerFilters(filters ...DockerFilter) client.Filters {
	f := make(client.Filters, len(filters))
	for _, filter := range filters {
		f.Add(filter.Term, filter.Values...)
	}
	return f
}

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

	reloadTrigger = Event{
		Type:            events.EventTypeDocker,
		Action:          events.ActionForceReload,
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

func (w DockerWatcher) Events(ctx context.Context) (<-chan Event, <-chan gperr.Error) {
	return w.EventsWithOptions(ctx, optionsDefault)
}

func (w DockerWatcher) EventsWithOptions(ctx context.Context, options DockerListOptions) (<-chan Event, <-chan gperr.Error) {
	eventCh := make(chan Event)
	errCh := make(chan gperr.Error)

	go func() {
		client, err := docker.NewClient(w.cfg)
		if err != nil {
			errCh <- gperr.Wrap(err, "docker watcher: failed to initialize client")
			return
		}

		defer func() {
			close(eventCh)
			close(errCh)
			client.Close()
		}()

		chs := client.Events(ctx, options)
		defer log.Debug().Str("host", client.DaemonHost()).Msg("docker watcher closed")
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-chs.Messages:
				w.handleEvent(msg, eventCh)
			case err := <-chs.Err:
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
				chs = client.Events(ctx, options)
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
		log.Debug().Err(err).Str("host", client.DaemonHost()).Msg("docker watcher: connection failed")
		return false
	}
	return true
}
