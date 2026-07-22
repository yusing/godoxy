package provider

import (
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/watcher"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/task"
)

type eventHandlerTestProviderImpl struct {
	err error
}

type eventHandlerTestDiagnostics struct {
	logger      zerolog.Logger
	discoveries [][]proxmox.Discovery
}

func (d *eventHandlerTestDiagnostics) LoadLogger() *zerolog.Logger {
	return &d.logger
}

func (d *eventHandlerTestDiagnostics) LogProxmoxDiscoveries(discoveries []proxmox.Discovery) {
	d.discoveries = append(d.discoveries, discoveries)
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

func TestEventHandlerCollectsOnlyMarkedProxmoxDiscoveries(t *testing.T) {
	handler := new(EventHandler)
	vmid := uint64(147)
	unmarked := &route.Route{
		Alias:   "rejected",
		Proxmox: &proxmox.NodeConfig{Node: "pve", VMID: &vmid},
	}
	marked := &route.Route{
		Alias:   "successful",
		Proxmox: &proxmox.NodeConfig{Node: "pve", VMID: &vmid},
	}
	marked.MarkProxmoxDiscovered(proxmox.DiscoveryResource)

	handler.recordProxmoxDiscovery(unmarked)
	handler.recordProxmoxDiscovery(marked)
	require.Equal(t, []proxmox.Discovery{{
		Kind:  proxmox.DiscoveryResource,
		Node:  "pve",
		Alias: "successful",
		VMID:  147,
	}}, handler.proxmoxDiscoveries)
}

func TestEventHandlerEmitsItsProxmoxDiscoveryBatch(t *testing.T) {
	diagnostics := &eventHandlerTestDiagnostics{logger: zerolog.Nop()}
	discoveries := []proxmox.Discovery{{
		Kind:  proxmox.DiscoveryResource,
		Node:  "pve",
		Alias: "radarr",
		VMID:  147,
	}}
	handler := &EventHandler{
		provider:           &Provider{diagnostics: diagnostics},
		proxmoxDiscoveries: discoveries,
	}

	handler.Log()
	require.Equal(t, [][]proxmox.Discovery{discoveries}, diagnostics.discoveries)
}
