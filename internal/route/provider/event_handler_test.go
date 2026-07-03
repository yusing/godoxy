package provider

import (
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/watcher"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/task"
)

type eventHandlerTestProviderImpl struct {
	err error
}

func (eventHandlerTestProviderImpl) String() string { return "test-provider" }

func (eventHandlerTestProviderImpl) ShortName() string { return "test" }

func (eventHandlerTestProviderImpl) IsExplicitOnly() bool { return false }

func (impl eventHandlerTestProviderImpl) loadRoutesImpl() (route.Routes, error) {
	return nil, impl.err
}

func (eventHandlerTestProviderImpl) NewWatcher() watcher.Watcher { return noopWatcher{} }

func (eventHandlerTestProviderImpl) Logger() *zerolog.Logger {
	logger := zerolog.Nop()
	return &logger
}

func TestEventHandlerKeepsExistingRoutesWhenForceReloadReturnsEmptyOnError(t *testing.T) {
	rootTask := task.RootTask("test", true)
	t.Cleanup(func() {
		rootTask.FinishAndWait("test finished")
	})

	providerErr := errors.New("unexpected EOF")
	p := &Provider{
		ProviderImpl: eventHandlerTestProviderImpl{err: providerErr},
		t:            routing.ProviderTypeDocker,
		routes: route.Routes{
			"app": {Alias: "app"},
		},
	}

	p.newEventHandler().Handle(rootTask, []watcher.Event{{
		Type:   watcherEvents.EventTypeDocker,
		Action: watcherEvents.ActionForceReload,
	}})

	currentRoute, ok := p.lockGetRoute("app")
	require.True(t, ok)
	require.NotNil(t, currentRoute)
	require.Equal(t, "app", currentRoute.Alias)
}

func TestEventHandlerShouldUpdateRouteOnForceReloadEvenWithoutMatchingContainerEvent(t *testing.T) {
	handler := &EventHandler{
		provider: &Provider{t: routing.ProviderTypeDocker},
	}

	shouldUpdate := handler.shouldUpdateRoute(true, []watcher.Event{{
		Type:      watcherEvents.EventTypeDocker,
		Action:    watcherEvents.ActionContainerDie,
		ActorID:   "other-id",
		ActorName: "other-name",
	}}, &route.Route{
		Metadata: route.Metadata{
			Container: &docker.Container{
				ContainerID:   "app-id",
				ContainerName: "app-name",
			},
		},
	})

	require.True(t, shouldUpdate)
}

func TestHasForceReload(t *testing.T) {
	require.True(t, hasForceReload([]watcher.Event{{
		Type:   watcherEvents.EventTypeDocker,
		Action: watcherEvents.ActionForceReload,
	}}))
	require.False(t, hasForceReload([]watcher.Event{{
		Type:   watcherEvents.EventTypeDocker,
		Action: watcherEvents.ActionContainerStart,
	}}))
}
