package provider

import (
	"context"
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

func (impl eventHandlerTestProviderImpl) loadRoutesImpl(context.Context) (route.Routes, error) {
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

	app := &route.Route{
		Container: &docker.Container{
			ContainerID:   "app-id",
			ContainerName: "app-name",
		},
	}
	shouldUpdate := handler.shouldUpdateRoute(true, []watcher.Event{{
		Type:      watcherEvents.EventTypeDocker,
		Action:    watcherEvents.ActionContainerDie,
		ActorID:   "other-id",
		ActorName: "other-name",
	}}, app, app)

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

func TestEventHandlerSkipsUnchangedFileRoutes(t *testing.T) {
	handler := &EventHandler{provider: &Provider{t: routing.ProviderTypeFile}}
	written := []watcher.Event{{Type: watcherEvents.EventTypeFile, Action: watcherEvents.ActionFileWritten}}
	withFP := func(fp string) *route.Route {
		r := &route.Route{}
		r.SetSourceFingerprint(fp)
		return r
	}

	require.False(t, handler.shouldUpdateRoute(false, written, withFP("a"), withFP("a")), "unchanged definition")
	require.True(t, handler.shouldUpdateRoute(false, written, withFP("a"), withFP("b")), "changed definition")
	require.True(t, handler.shouldUpdateRoute(false, written, withFP(""), withFP("")), "no fingerprint: update as before")
	require.True(t, handler.shouldUpdateRoute(true, written, withFP("a"), withFP("a")), "force reload restarts everything")
}

func TestSourceFingerprints(t *testing.T) {
	base := []byte(`
x-common: &common
  scheme: http
alpha:
  <<: *common
  host: 10.0.0.1
  port: "80"
beta:
  host: 10.0.0.2
`)
	reordered := []byte(`
beta:
  host: 10.0.0.2
x-common: &common
  scheme: http
alpha:
  port: "80"   # comments and key order do not matter
  host: 10.0.0.1
  <<: *common
`)
	anchorChanged := []byte(`
x-common: &common
  scheme: https
alpha:
  <<: *common
  host: 10.0.0.1
  port: "80"
beta:
  host: 10.0.0.2
`)
	a, b, c := sourceFingerprints(base), sourceFingerprints(reordered), sourceFingerprints(anchorChanged)
	require.NotEmpty(t, a["alpha"])
	require.Equal(t, a["alpha"], b["alpha"])
	require.Equal(t, a["beta"], b["beta"])
	require.NotEqual(t, a["alpha"], c["alpha"], "a change in a shared anchor changes the routes using it")
	require.Equal(t, a["beta"], c["beta"])
	require.Nil(t, sourceFingerprints([]byte("alpha: [")))
}
