package provider

import (
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/watcher"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/task"
)

type EventHandler struct {
	provider *Provider

	errs               gperr.Builder
	activation         routing.ProviderActivation
	proxmoxDiscoveries []proxmox.Discovery
}

func (p *Provider) newEventHandler() *EventHandler {
	return &EventHandler{
		provider: p,
		errs:     gperr.NewBuilder("event errors"),
	}
}

func (handler *EventHandler) Handle(parent task.Parent, events []watcher.Event) {
	oldRoutes := handler.provider.lockCloneRoutes()
	forceReload := hasForceReload(events)

	newRoutes, err := handler.provider.loadRoutes(parent.Context())
	handler.activation = handler.provider.preparation
	handler.activation.FailedRoutes = append([]routing.RouteActivationIssue(nil), handler.activation.FailedRoutes...)
	handler.activation.AttemptedRoutes = len(newRoutes)
	defer func() {
		p := handler.provider
		p.activationMu.Lock()
		defer p.activationMu.Unlock()
		handler.activation.ActiveRoutes = p.NumRoutes()
		handler.activation.EventLoopReady = p.activation.EventLoopReady
		handler.activation.InfrastructureError = gperr.Join(handler.activation.InfrastructureError, p.watcherErr)
		p.activation = handler.activation
	}()
	if err != nil {
		handler.errs.Add(err)
		if len(newRoutes) == 0 {
			return
		}
	}

	for k, oldr := range oldRoutes {
		newr, ok := newRoutes[k]
		switch {
		case !ok:
			// A partial file load cannot distinguish invalid aliases from deletions.
			// Docker listings can still confirm removals despite sibling errors.
			if err == nil || handler.provider.GetType() != routing.ProviderTypeFile {
				handler.Remove(oldr)
			}
		case handler.shouldUpdateRoute(forceReload, events, oldr, newr):
			handler.Update(parent, oldr, newr)
		}
	}
	for k, newr := range newRoutes {
		if _, ok := oldRoutes[k]; !ok {
			handler.Add(parent, newr)
		}
	}
}

// shouldUpdateRoute reports whether newr must replace the running oldr.
//
// A file event matches every route in the file. Restarting a route removes it
// from the entrypoint until its replacement starts and resets its health
// state, so routes whose raw definition did not change are kept running. A
// force reload still restarts everything.
func (handler *EventHandler) shouldUpdateRoute(forceReload bool, events []watcher.Event, oldr, newr *route.Route) bool {
	if forceReload {
		return true
	}
	if !handler.matchAny(events, newr) {
		return false
	}
	if handler.provider.GetType() == routing.ProviderTypeFile && sameSource(oldr, newr) {
		return false
	}
	return true
}

func sameSource(oldr, newr *route.Route) bool {
	fp := oldr.SourceFingerprint()
	return fp != "" && fp == newr.SourceFingerprint()
}

func hasForceReload(events []watcher.Event) bool {
	for _, event := range events {
		if event.Action == watcherEvents.ActionForceReload {
			return true
		}
	}
	return false
}

func (handler *EventHandler) matchAny(events []watcher.Event, rt *route.Route) bool {
	for _, event := range events {
		if handler.match(event, rt) {
			return true
		}
	}
	return false
}

func (handler *EventHandler) match(event watcher.Event, rt *route.Route) bool {
	switch handler.provider.GetType() {
	case routing.ProviderTypeDocker, routing.ProviderTypeAgent:
		return rt.Container.ContainerID == event.ActorID ||
			rt.Container.ContainerName == event.ActorName
	case routing.ProviderTypeFile:
		return true
	}
	// should never happen
	return false
}

func (handler *EventHandler) Add(parent task.Parent, route *route.Route) {
	err := handler.provider.startRoute(parent, route)
	if err != nil {
		handler.errs.AddSubjectf(err, "add")
		handler.activation.FailedRoutes = append(handler.activation.FailedRoutes, routing.RouteActivationIssue{Route: route.Alias, Err: gperr.Wrap(err)})
		return
	}
	handler.recordProxmoxDiscovery(route)
}

func (handler *EventHandler) Remove(route *route.Route) {
	route.FinishAndWait("route removed")
}

func (handler *EventHandler) Update(parent task.Parent, oldRoute *route.Route, newRoute *route.Route) {
	oldRoute.FinishAndWait("route update")
	err := handler.provider.startRoute(parent, newRoute)
	if err != nil {
		handler.errs.AddSubjectf(err, "update")
		handler.activation.FailedRoutes = append(handler.activation.FailedRoutes, routing.RouteActivationIssue{Route: newRoute.Alias, Err: gperr.Wrap(err)})
		return
	}
	handler.recordProxmoxDiscovery(newRoute)
}

func (handler *EventHandler) Log() {
	if err := handler.errs.Error(); err != nil {
		handler.provider.Logger().Err(err).Msg("provider event failed")
	}
	if len(handler.proxmoxDiscoveries) == 0 {
		return
	}
	if handler.provider.diagnostics == nil {
		return
	}
	handler.provider.diagnostics.LogProxmoxDiscoveries(handler.proxmoxDiscoveries)
}

func (handler *EventHandler) recordProxmoxDiscovery(rt *route.Route) {
	if discovery, ok := rt.ProxmoxDiscovery(); ok {
		handler.proxmoxDiscoveries = append(handler.proxmoxDiscoveries, discovery)
	}
}
